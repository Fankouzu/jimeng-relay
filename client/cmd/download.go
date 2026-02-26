package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jimeng-relay/client/internal/jimeng"
	"github.com/spf13/cobra"
)

type downloadFlagValues struct {
	taskID    string
	dir       string
	overwrite bool
}

var downloadFlags downloadFlagValues

type downloadResult struct {
	TaskID    string   `json:"task_id"`
	Status    string   `json:"status"`
	ImageURLs []string `json:"image_urls"`
	Files     []string `json:"files"`
}

var downloadHTTPClient = &http.Client{Timeout: 60 * time.Second}

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download task result",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClientAndFormatter(cmd)
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		dir := strings.TrimSpace(downloadFlags.dir)
		if dir == "" {
			return fmt.Errorf("--dir is required")
		}

		resp, err := client.GetResult(ctx, jimeng.GetResultRequest{TaskID: downloadFlags.taskID})
		if err != nil {
			return err
		}
		if resp.Status != jimeng.StatusDone {
			return fmt.Errorf("task is not done: status=%s", resp.Status)
		}

		var files []string
		switch {
		case len(resp.ImageURLs) > 0:
			files, err = downloadImagesToDir(ctx, resp.ImageURLs, dir, downloadFlags.overwrite)
		case len(resp.BinaryDataBase64) > 0:
			files, err = decodeBase64ImagesToDir(ctx, resp.BinaryDataBase64, dir, downloadFlags.overwrite)
		default:
			return fmt.Errorf("task has no image URLs or binary_data_base64")
		}
		if err != nil {
			return err
		}

		res := downloadResult{
			TaskID:    downloadFlags.taskID,
			Status:    string(resp.Status),
			ImageURLs: resp.ImageURLs,
			Files:     files,
		}
		out, err := formatDownloadResult(res)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), out)
		return nil
	},
}

func formatDownloadResult(res downloadResult) (string, error) {
	f := strings.TrimSpace(rootFlags.format)
	if f == "" {
		f = "text"
	}

	switch f {
	case "json":
		b, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	case "text":
		parts := make([]string, 0, 4)
		parts = append(parts, fmt.Sprintf("TaskID=%s", res.TaskID))
		if res.Status != "" {
			parts = append(parts, fmt.Sprintf("Status=%s", res.Status))
		}
		if len(res.Files) > 0 {
			parts = append(parts, fmt.Sprintf("Files=%s", strings.Join(res.Files, ",")))
		}
		return strings.Join(parts, " "), nil
	default:
		return "", fmt.Errorf("invalid --format: %q (supported: text|json)", f)
	}
}

func downloadImagesToDir(ctx context.Context, imageURLs []string, dir string, overwrite bool) ([]string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("download dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create download dir failed: %w", err)
	}

	files := make([]string, 0, len(imageURLs))
	for i, raw := range imageURLs {
		filePath, err := downloadOne(ctx, strings.TrimSpace(raw), dir, i, overwrite)
		if err != nil {
			return nil, err
		}
		files = append(files, filePath)
	}
	return files, nil
}

func decodeBase64ImagesToDir(ctx context.Context, base64Images []string, dir string, overwrite bool) ([]string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("download dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create download dir failed: %w", err)
	}

	files := make([]string, 0, len(base64Images))
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

		fileName := fmt.Sprintf("image-%d.png", i+1)
		filePath := filepath.Join(dir, fileName)

		flags := os.O_CREATE | os.O_WRONLY
		if overwrite {
			flags |= os.O_TRUNC
		} else {
			flags |= os.O_EXCL
		}

		out, err := os.OpenFile(filePath, flags, 0o644)
		if err != nil {
			return nil, fmt.Errorf("open output file failed: %w", err)
		}
		if _, err := out.Write(data); err != nil {
			if closeErr := out.Close(); closeErr != nil {
				return nil, fmt.Errorf("write image content failed: %w (close output file failed: %v)", err, closeErr)
			}
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

func downloadOne(ctx context.Context, rawURL string, dir string, index int, overwrite bool) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("image url is empty")
	}
	fileName, err := fileNameFromURL(rawURL, index)
	if err != nil {
		return "", err
	}
	filePath := filepath.Join(dir, fileName)

	flags := os.O_CREATE | os.O_WRONLY
	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}

	out, err := os.OpenFile(filePath, flags, 0o644)
	if err != nil {
		return "", fmt.Errorf("open output file failed: %w", err)
	}
	defer out.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("create download request failed: %w", err)
	}
	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download request returned status=%d", resp.StatusCode)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", fmt.Errorf("write image content failed: %w", err)
	}

	return filePath, nil
}

func fileNameFromURL(rawURL string, index int) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse image url failed: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported image url scheme: %s", u.Scheme)
	}
	base := path.Base(u.Path)
	if base == "" || base == "." || base == "/" || base == ".." {
		base = fmt.Sprintf("image-%d", index+1)
	}
	return base, nil
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	downloadCmd.Flags().StringVar(&downloadFlags.taskID, "task-id", "", "Task ID")
	downloadCmd.Flags().StringVar(&downloadFlags.dir, "dir", "", "Download directory")
	downloadCmd.Flags().BoolVar(&downloadFlags.overwrite, "overwrite", false, "Overwrite existing files")
	if err := downloadCmd.MarkFlagRequired("task-id"); err != nil {
		panic(err)
	}
	if err := downloadCmd.MarkFlagRequired("dir"); err != nil {
		panic(err)
	}
}
