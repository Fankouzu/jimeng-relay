package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jimeng-relay/client/internal/jimeng"
	"github.com/jimeng-relay/client/internal/output"
	"github.com/spf13/cobra"
)

type submitFlagValues struct {
	prompt      string
	imageURLs   []string
	imageFiles  []string
	resolution  string
	count       int
	quality     string
	width       int
	height      int
	scale       int
	forceSingle bool
	minRatio    string
	maxRatio    string

	wait        bool
	waitTimeout string
	downloadDir string
	overwrite   bool
}

var submitFlags submitFlagValues

const maxInputImageFileBytes = 5 * 1024 * 1024

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit a task",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, formatter, err := newClientAndFormatter(cmd)
		if err != nil {
			return err
		}

		if submitFlags.count < 1 || submitFlags.count > 4 {
			return fmt.Errorf("--count must be in range [1,4]")
		}

		qualityMode, err := normalizeQualityMode(submitFlags.quality)
		if err != nil {
			return err
		}

		width := submitFlags.width
		height := submitFlags.height
		if width == 0 && height == 0 {
			width, height, err = parseResolution(submitFlags.resolution)
			if err != nil {
				return err
			}
		}

		binaryDataBase64, err := loadImageFilesAsBase64(submitFlags.imageFiles)
		if err != nil {
			return err
		}

		// Process image URLs - detect local paths and convert to base64
		imageURLs, urlBase64, err := processImageURLs(submitFlags.imageURLs)
		if err != nil {
			return err
		}

		// Merge base64 data from --image-file and local paths in --image-url
		allBase64 := append(binaryDataBase64, urlBase64...)

		req := jimeng.SubmitRequest{
			Prompt:           submitFlags.prompt,
			ImageURLs:        imageURLs,
			BinaryDataBase64: allBase64,
			Width:            width,
			Height:           height,
			Scale:            submitFlags.scale,
			ForceSingle:      submitFlags.forceSingle,
			MinRatio:         submitFlags.minRatio,
			MaxRatio:         submitFlags.maxRatio,
		}

		if (width > 0) != (height > 0) {
			return fmt.Errorf("--width and --height must be set together")
		}

		if !flagChanged(cmd, "scale") {
			if qualityMode == "quality" {
				req.Scale = 1
			} else {
				req.Scale = 0
			}
		}

		if err := jimeng.ValidateSubmitRequest(&req); err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		downloadDir := strings.TrimSpace(submitFlags.downloadDir)
		shouldWait := submitFlags.wait || downloadDir != ""

		if submitFlags.count > 1 {
			if shouldWait {
				results := make([]*jimeng.FlowResult, 0, submitFlags.count)
				for i := 0; i < submitFlags.count; i++ {
					// Add delay between requests to avoid API concurrent limit
					if i > 0 {
						time.Sleep(500 * time.Millisecond)
					}
					opts := jimeng.FlowOptions{Wait: true, DownloadDir: downloadDir, Overwrite: submitFlags.overwrite}
					if raw := strings.TrimSpace(submitFlags.waitTimeout); raw != "" {
						d, err := time.ParseDuration(raw)
						if err != nil {
							return fmt.Errorf("invalid --wait-timeout: %w", err)
						}
						opts.Timeout = d
					}

					res, err := client.GenerateImage(ctx, req, opts)
					if err != nil {
						return fmt.Errorf("generation %d/%d failed: %w", i+1, submitFlags.count, err)
					}
					results = append(results, res)
				}

				out, err := formatFlowBatchResult(results)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), out)
				return nil
			}

			responses := make([]*jimeng.SubmitResponse, 0, submitFlags.count)
			for i := 0; i < submitFlags.count; i++ {
				// Add delay between requests to avoid API concurrent limit
				if i > 0 {
					time.Sleep(500 * time.Millisecond)
				}
				resp, err := client.SubmitTask(ctx, req)
				if err != nil {
					return fmt.Errorf("generation %d/%d failed: %w", i+1, submitFlags.count, err)
				}
				responses = append(responses, resp)
			}

			out, err := formatSubmitBatchResult(formatter, responses)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		}

		if shouldWait {
			opts := jimeng.FlowOptions{
				Wait:        true,
				DownloadDir: downloadDir,
				Overwrite:   submitFlags.overwrite,
			}
			if raw := strings.TrimSpace(submitFlags.waitTimeout); raw != "" {
				d, err := time.ParseDuration(raw)
				if err != nil {
					return fmt.Errorf("invalid --wait-timeout: %w", err)
				}
				opts.Timeout = d
			}

			res, err := client.GenerateImage(ctx, req, opts)
			if err != nil {
				return err
			}
			out, err := formatFlowResult(res)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		}

		resp, err := client.SubmitTask(ctx, req)
		if err != nil {
			return err
		}
		out, err := formatter.FormatSubmitResponse(resp)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), out)
		return nil
	},
}

func loadImageFilesAsBase64(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		path := strings.TrimSpace(p)
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat --image-file %q failed: %w", path, err)
		}
		if info.Size() <= 0 {
			return nil, fmt.Errorf("--image-file %q is empty", path)
		}
		if info.Size() > maxInputImageFileBytes {
			return nil, fmt.Errorf("--image-file %q exceeds max size %d bytes", path, maxInputImageFileBytes)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read --image-file %q failed: %w", path, err)
		}
		out = append(out, base64.StdEncoding.EncodeToString(data))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--image-file provided but no valid file path found")
	}
	return out, nil
}

// processImageURLs processes image URLs, detecting local paths and converting them to base64
func processImageURLs(urls []string) ([]string, []string, error) {
	if len(urls) == 0 {
		return nil, nil, nil
	}

	var remoteURLs []string
	var base64Data []string

	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}

		// Check if it's a local path (starts with ./, /, or file exists)
		if strings.HasPrefix(u, "./") || strings.HasPrefix(u, "/") || strings.HasPrefix(u, "../") {
			// It's a local path, read and convert to base64
			info, err := os.Stat(u)
			if err != nil {
				return nil, nil, fmt.Errorf("stat --image-url %q failed: %w", u, err)
			}
			if info.Size() <= 0 {
				return nil, nil, fmt.Errorf("--image-url %q is empty", u)
			}
			if info.Size() > maxInputImageFileBytes {
				return nil, nil, fmt.Errorf("--image-url %q exceeds max size %d bytes", u, maxInputImageFileBytes)
			}
			data, err := os.ReadFile(u)
			if err != nil {
				return nil, nil, fmt.Errorf("read --image-url %q failed: %w", u, err)
			}
			base64Data = append(base64Data, base64.StdEncoding.EncodeToString(data))
		} else if _, err := os.Stat(u); err == nil {
			// File exists, treat as local path
			info, err := os.Stat(u)
			if err != nil {
				return nil, nil, fmt.Errorf("stat --image-url %q failed: %w", u, err)
			}
			if info.Size() <= 0 {
				return nil, nil, fmt.Errorf("--image-url %q is empty", u)
			}
			if info.Size() > maxInputImageFileBytes {
				return nil, nil, fmt.Errorf("--image-url %q exceeds max size %d bytes", u, maxInputImageFileBytes)
			}
			data, err := os.ReadFile(u)
			if err != nil {
				return nil, nil, fmt.Errorf("read --image-url %q failed: %w", u, err)
			}
			base64Data = append(base64Data, base64.StdEncoding.EncodeToString(data))
		} else {
			// It's a remote URL
			remoteURLs = append(remoteURLs, u)
		}
	}

	return remoteURLs, base64Data, nil
}

func normalizeQualityMode(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "speed", nil
	}
	switch strings.ToLower(v) {
	case "speed", "fast", "speed-first", "速度优先":
		return "speed", nil
	case "quality", "high", "quality-first", "质量优先":
		return "quality", nil
	default:
		return "", fmt.Errorf("invalid --quality: %q (supported: speed|quality|速度优先|质量优先)", raw)
	}
}

func parseResolution(raw string) (int, int, error) {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" {
		return 0, 0, fmt.Errorf("--resolution must not be empty")
	}
	parts := strings.Split(v, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid --resolution: %q, expected <width>x<height>", raw)
	}
	w, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid --resolution width: %w", err)
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid --resolution height: %w", err)
	}
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("--resolution width/height must be positive")
	}
	return w, h, nil
}

func formatSubmitBatchResult(formatter *output.Formatter, responses []*jimeng.SubmitResponse) (string, error) {
	f := strings.TrimSpace(rootFlags.format)
	if f == "" {
		f = "text"
	}
	if f == "json" {
		b, err := json.MarshalIndent(responses, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	lines := make([]string, 0, len(responses))
	for i, r := range responses {
		line, err := formatter.FormatSubmitResponse(r)
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("[%d] %s", i+1, line))
	}
	return strings.Join(lines, "\n"), nil
}

func formatFlowBatchResult(results []*jimeng.FlowResult) (string, error) {
	f := strings.TrimSpace(rootFlags.format)
	if f == "" {
		f = "text"
	}
	if f == "json" {
		b, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	lines := make([]string, 0, len(results))
	for i, r := range results {
		line, err := formatFlowResult(r)
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("[%d] %s", i+1, line))
	}
	return strings.Join(lines, "\n"), nil
}

func formatFlowResult(res *jimeng.FlowResult) (string, error) {
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
		if res == nil {
			return "", nil
		}
		parts := make([]string, 0, 4)
		parts = append(parts, fmt.Sprintf("TaskID=%s", res.TaskID))
		if res.Status != "" {
			parts = append(parts, fmt.Sprintf("Status=%s", res.Status))
		}
		if len(res.ImageURLs) > 0 {
			parts = append(parts, fmt.Sprintf("ImageURLs=%s", strings.Join(res.ImageURLs, ",")))
		}
		if len(res.LocalFiles) > 0 {
			parts = append(parts, fmt.Sprintf("LocalFiles=%s", strings.Join(res.LocalFiles, ",")))
		}
		return strings.Join(parts, " "), nil
	default:
		return "", fmt.Errorf("invalid --format: %q (supported: text|json)", f)
	}
}

func init() {
	rootCmd.AddCommand(submitCmd)

	submitCmd.Flags().StringVar(&submitFlags.prompt, "prompt", "", "Prompt text")
	submitCmd.Flags().StringArrayVar(&submitFlags.imageURLs, "image-url", nil, "Input image URL (repeatable)")
	submitCmd.Flags().StringArrayVar(&submitFlags.imageFiles, "image-file", nil, "Input local image file path (repeatable, auto base64)")
	submitCmd.Flags().StringVar(&submitFlags.resolution, "resolution", "2048x2048", "Image resolution, format <width>x<height>, default 2048x2048")
	submitCmd.Flags().IntVar(&submitFlags.count, "count", 1, "Number of images to generate, range 1-4")
	submitCmd.Flags().StringVar(&submitFlags.quality, "quality", "speed", "Quality preset: speed|quality (also supports 速度优先|质量优先)")
	submitCmd.Flags().IntVar(&submitFlags.width, "width", 0, "Image width (must pair with --height)")
	submitCmd.Flags().IntVar(&submitFlags.height, "height", 0, "Image height (must pair with --width)")
	submitCmd.Flags().IntVar(&submitFlags.scale, "scale", 0, "Scale factor (0 to omit)")
	submitCmd.Flags().BoolVar(&submitFlags.forceSingle, "force-single", false, "Force single image")
	submitCmd.Flags().StringVar(&submitFlags.minRatio, "min-ratio", "", "Min ratio, e.g. 1/2 or 0.5")
	submitCmd.Flags().StringVar(&submitFlags.maxRatio, "max-ratio", "", "Max ratio, e.g. 2/1 or 2")

	submitCmd.Flags().BoolVar(&submitFlags.wait, "wait", false, "Wait for task completion")
	submitCmd.Flags().StringVar(&submitFlags.waitTimeout, "wait-timeout", "", "Wait timeout duration, e.g. 60s, 5m")
	submitCmd.Flags().StringVar(&submitFlags.downloadDir, "download-dir", "", "If set, download result images into this directory (implies --wait)")
	submitCmd.Flags().BoolVar(&submitFlags.overwrite, "overwrite", false, "Overwrite existing files when downloading")

	if err := submitCmd.MarkFlagRequired("prompt"); err != nil {
		panic(err)
	}
}
