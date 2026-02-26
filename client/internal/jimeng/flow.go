package jimeng

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type FlowOptions struct {
	Wait        bool
	DownloadDir string
	Overwrite   bool
	Timeout     time.Duration
	Format      string
}

type FlowResult struct {
	TaskID     string
	Status     TaskStatus
	ImageURLs  []string
	LocalFiles []string
}

var imageDownloadHTTPClient = &http.Client{Timeout: 60 * time.Second}

func (c *Client) GenerateImage(ctx context.Context, req SubmitRequest, opts FlowOptions) (*FlowResult, error) {
	submitResp, err := c.SubmitTask(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("submit task failed: %w", err)
	}

	taskID := strings.TrimSpace(submitResp.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("submit task returned empty task_id")
	}

	result := &FlowResult{TaskID: taskID}
	if !opts.Wait {
		return result, nil
	}

	waitResult, err := c.Wait(ctx, taskID, WaitOptions{Timeout: opts.Timeout})
	if err != nil {
		return nil, fmt.Errorf("task_id=%s: wait failed: %w", taskID, err)
	}

	result.Status = waitResult.FinalStatus
	result.ImageURLs = append(result.ImageURLs, waitResult.ImageURLs...)

	if waitResult.FinalStatus != StatusDone {
		return nil, fmt.Errorf("task_id=%s: task finished with status=%s", taskID, waitResult.FinalStatus)
	}

	// Post-terminal re-fetch strategy: sometimes status is 'done' but payload is not yet available.
	if len(waitResult.ImageURLs) == 0 && len(waitResult.BinaryDataBase64) == 0 {
		for i := 0; i < 3; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}

			resp, err := c.GetResult(ctx, GetResultRequest{TaskID: taskID})
			if err == nil && (len(resp.ImageURLs) > 0 || len(resp.BinaryDataBase64) > 0) {
				waitResult.ImageURLs = resp.ImageURLs
				waitResult.BinaryDataBase64 = resp.BinaryDataBase64
				result.ImageURLs = append([]string{}, resp.ImageURLs...)
				break
			}
		}
	}

	if strings.TrimSpace(opts.DownloadDir) == "" {
		return result, nil
	}

	var localFiles []string
	switch {
	case len(waitResult.ImageURLs) > 0:
		localFiles, err = downloadImages(ctx, taskID, waitResult.ImageURLs, opts)
	case len(waitResult.BinaryDataBase64) > 0:
		localFiles, err = downloadImagesFromBase64(ctx, taskID, waitResult.BinaryDataBase64, opts)
	default:
		return nil, fmt.Errorf("task_id=%s: task is done but no image URLs or binary_data_base64 returned", taskID)
	}
	if err != nil {
		return nil, fmt.Errorf("task_id=%s: download images failed: %w", taskID, err)
	}

	result.LocalFiles = localFiles
	return result, nil
}

func downloadImagesFromBase64(ctx context.Context, taskID string, base64Images []string, opts FlowOptions) ([]string, error) {
	dir := strings.TrimSpace(opts.DownloadDir)
	if dir == "" {
		return nil, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create download dir failed: %w", err)
	}

	format := strings.TrimPrefix(strings.TrimSpace(opts.Format), ".")
	if format == "" {
		format = "png"
	}

	files := make([]string, 0, len(base64Images))
	taskKey := sanitizeTaskID(taskID)
	for i, raw := range base64Images {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil, fmt.Errorf("binary_data_base64 is empty at index=%d", i)
		}

		payload := raw
		if strings.HasPrefix(payload, "data:") {
			if comma := strings.Index(payload, ","); comma >= 0 {
				payload = payload[comma+1:]
			}
		}
		payload = stripBase64Whitespace(payload)

		data, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			data, err = base64.RawStdEncoding.DecodeString(payload)
		}
		if err != nil {
			data, err = base64.URLEncoding.DecodeString(payload)
		}
		if err != nil {
			data, err = base64.RawURLEncoding.DecodeString(payload)
		}
		if err != nil {
			return nil, fmt.Errorf("decode base64 image index=%d failed: %w", i, err)
		}

		fileName := fmt.Sprintf("%s-image-%d.%s", taskKey, i+1, format)
		filePath := filepath.Join(dir, fileName)

		flags := os.O_CREATE | os.O_WRONLY
		if opts.Overwrite {
			flags |= os.O_TRUNC
		} else {
			flags |= os.O_EXCL
		}

		out, err := os.OpenFile(filePath, flags, 0o644)
		if err != nil {
			return nil, fmt.Errorf("open output file failed: %w", err)
		}
		if _, err := out.Write(data); err != nil {
			_ = out.Close()
			return nil, fmt.Errorf("write image content failed: %w", err)
		}
		if err := out.Close(); err != nil {
			return nil, fmt.Errorf("close output file failed: %w", err)
		}

		files = append(files, filePath)
	}

	return files, nil
}

func stripBase64Whitespace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, s)
}

func downloadImages(ctx context.Context, taskID string, imageURLs []string, opts FlowOptions) ([]string, error) {
	dir := strings.TrimSpace(opts.DownloadDir)
	if dir == "" {
		return nil, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create download dir failed: %w", err)
	}

	files := make([]string, 0, len(imageURLs))
	for i, imageURL := range imageURLs {
		filePath, err := downloadImage(ctx, taskID, strings.TrimSpace(imageURL), i, opts)
		if err != nil {
			return nil, fmt.Errorf("download image index=%d failed: %w", i, err)
		}
		files = append(files, filePath)
	}

	return files, nil
}

func downloadImage(ctx context.Context, taskID string, imageURL string, index int, opts FlowOptions) (string, error) {
	if imageURL == "" {
		return "", fmt.Errorf("image url is empty")
	}

	u, err := url.Parse(imageURL)
	if err != nil {
		return "", fmt.Errorf("parse image url failed: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported image url scheme: %s", u.Scheme)
	}

	fileName, err := buildFileName(taskID, imageURL, index, opts.Format)
	if err != nil {
		return "", fmt.Errorf("build file name failed: %w", err)
	}

	filePath := filepath.Join(strings.TrimSpace(opts.DownloadDir), fileName)
	flags := os.O_CREATE | os.O_WRONLY
	if opts.Overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}

	output, err := os.OpenFile(filePath, flags, 0o644)
	if err != nil {
		return "", fmt.Errorf("open output file failed: %w", err)
	}
	defer output.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("create download request failed: %w", err)
	}

	resp, err := imageDownloadHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download request returned status=%d", resp.StatusCode)
	}

	if _, err := io.Copy(output, resp.Body); err != nil {
		return "", fmt.Errorf("write image content failed: %w", err)
	}

	return filePath, nil
}

func buildFileName(taskID string, rawURL string, index int, format string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse image url failed: %w", err)
	}

	taskKey := sanitizeTaskID(taskID)

	base := path.Base(u.Path)
	if base == "." || base == "/" || base == "" {
		base = fmt.Sprintf("image-%d", index+1)
	}

	cleanFormat := strings.TrimPrefix(strings.TrimSpace(format), ".")
	if cleanFormat == "" {
		return fmt.Sprintf("%s-%s", taskKey, base), nil
	}

	ext := "." + cleanFormat
	nameWithoutExt := strings.TrimSuffix(base, path.Ext(base))
	if nameWithoutExt == "" {
		nameWithoutExt = fmt.Sprintf("image-%d", index+1)
	}

	return fmt.Sprintf("%s-%s%s", taskKey, nameWithoutExt, ext), nil
}

func sanitizeTaskID(taskID string) string {
	key := strings.TrimSpace(taskID)
	if key == "" {
		return "task"
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, key)
}

func (c *Client) DownloadVideo(ctx context.Context, taskID string, videoURL string, opts FlowOptions) (string, error) {
	dir := strings.TrimSpace(opts.DownloadDir)
	if dir == "" {
		return "", fmt.Errorf("download dir is required")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create download dir failed: %w", err)
	}

	if videoURL == "" {
		return "", fmt.Errorf("video url is empty")
	}

	u, err := url.Parse(videoURL)
	if err != nil {
		return "", fmt.Errorf("parse video url failed: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported video url scheme: %s", u.Scheme)
	}

	taskKey := sanitizeTaskID(taskID)
	base := path.Base(u.Path)
	if base == "." || base == "/" || base == "" {
		base = "video.mp4"
	}

	// Ensure deterministic naming: <task_id>-<original_name>
	fileName := fmt.Sprintf("%s-%s", taskKey, base)
	filePath := filepath.Join(dir, fileName)

	flags := os.O_CREATE | os.O_WRONLY
	if opts.Overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}

	output, err := os.OpenFile(filePath, flags, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("file already exists: %s (use --overwrite to replace)", filePath)
		}
		return "", fmt.Errorf("open output file failed: %w", err)
	}
	defer output.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoURL, nil)
	if err != nil {
		return "", fmt.Errorf("create download request failed: %w", err)
	}

	resp, err := imageDownloadHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("download failed: status=%d (URL might be expired or invalid)", resp.StatusCode)
		}
		return "", fmt.Errorf("download request returned status=%d", resp.StatusCode)
	}

	if _, err := io.Copy(output, resp.Body); err != nil {
		return "", fmt.Errorf("write video content failed: %w", err)
	}

	return filePath, nil
}
