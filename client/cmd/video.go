package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jimeng-relay/client/internal/api"
	"github.com/jimeng-relay/client/internal/jimeng"
	"github.com/jimeng-relay/client/internal/output"
	"github.com/spf13/cobra"
)

type videoSubmitFlagValues struct {
	preset         string
	prompt         string
	frames         int
	aspectRatio    string
	imageURL       string
	template       string
	cameraStrength string
}

type videoQueryFlagValues struct {
	taskID string
	preset string
}

type videoWaitFlagValues struct {
	taskID   string
	preset   string
	interval string
	timeout  string
}

type videoDownloadFlagValues struct {
	taskID    string
	preset    string
	dir       string
	overwrite bool
}

var (
	videoSubmitFlags   videoSubmitFlagValues
	videoQueryFlags    videoQueryFlagValues
	videoWaitFlags     videoWaitFlagValues
	videoDownloadFlags videoDownloadFlagValues
)

var videoCmd = &cobra.Command{
	Use:   "video",
	Short: "Video generation commands",
	Long:  `Commands for JiMeng Video 3.0 generation, including submit, query, wait and download.`,
}

var videoSubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit a video generation task",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, formatter, err := newClientAndFormatter(cmd)
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		req, err := buildVideoSubmitRequest()
		if err != nil {
			return err
		}

		resp, err := client.SubmitVideoTask(ctx, req)
		if err != nil {
			return err
		}

		out, err := formatVideoSubmitResponse(formatter, resp)
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), out)
		return nil
	},
}

type videoSubmitResult struct {
	TaskID      string          `json:"task_id"`
	Preset      api.VideoPreset `json:"preset"`
	ReqKey      string          `json:"req_key"`
	Code        int             `json:"code"`
	Status      int             `json:"status"`
	Message     string          `json:"message"`
	RequestID   string          `json:"request_id"`
	TimeElapsed int             `json:"time_elapsed"`
}

type videoQueryResult struct {
	TaskID    string          `json:"task_id"`
	Preset    api.VideoPreset `json:"preset"`
	ReqKey    string          `json:"req_key"`
	Status    string          `json:"status"`
	VideoURL  string          `json:"video_url"`
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	RequestID string          `json:"request_id"`
}

type videoWaitRequest struct {
	TaskID  string
	Preset  api.VideoPreset
	Options jimeng.WaitOptions
}

type videoWaitResult struct {
	TaskID    string          `json:"task_id"`
	Preset    api.VideoPreset `json:"preset"`
	Status    string          `json:"status"`
	VideoURL  string          `json:"video_url"`
	PollCount int             `json:"poll_count"`
}

func buildVideoSubmitRequest() (jimeng.VideoSubmitRequest, error) {
	preset := api.VideoPreset(strings.TrimSpace(videoSubmitFlags.preset))
	prompt := strings.TrimSpace(videoSubmitFlags.prompt)
	if prompt == "" {
		return jimeng.VideoSubmitRequest{}, fmt.Errorf("--prompt is required")
	}

	if videoSubmitFlags.frames < 0 {
		return jimeng.VideoSubmitRequest{}, fmt.Errorf("--frames must be positive")
	}

	imageURLs, err := parseVideoImageURLs(videoSubmitFlags.imageURL)
	if err != nil {
		return jimeng.VideoSubmitRequest{}, err
	}

	cameraStrength, err := normalizeVideoCameraStrength(videoSubmitFlags.cameraStrength)
	if err != nil {
		return jimeng.VideoSubmitRequest{}, err
	}

	req := jimeng.VideoSubmitRequest{
		Preset:         preset,
		Prompt:         prompt,
		Frames:         videoSubmitFlags.frames,
		AspectRatio:    strings.TrimSpace(videoSubmitFlags.aspectRatio),
		ImageURLs:      imageURLs,
		TemplateID:     strings.TrimSpace(videoSubmitFlags.template),
		CameraStrength: cameraStrength,
	}

	switch preset {
	case api.VideoPresetT2V720, api.VideoPresetT2V1080:
		if req.Frames == 0 {
			req.Frames = 121
		}
		if req.AspectRatio == "" {
			req.AspectRatio = "16:9"
		}
		if len(req.ImageURLs) > 0 {
			return jimeng.VideoSubmitRequest{}, fmt.Errorf("--image-url is not allowed for %q", preset)
		}
	case api.VideoPresetI2VFirst:
		if len(req.ImageURLs) == 0 {
			return jimeng.VideoSubmitRequest{}, fmt.Errorf("--image-url is required for %q", preset)
		}
	case api.VideoPresetI2VFirstTail:
		if len(req.ImageURLs) != 2 {
			return jimeng.VideoSubmitRequest{}, fmt.Errorf("--image-url for %q requires exactly 2 URLs (comma-separated)", preset)
		}
	case api.VideoPresetI2VRecamera:
		if len(req.ImageURLs) == 0 {
			return jimeng.VideoSubmitRequest{}, fmt.Errorf("--image-url is required for %q", preset)
		}
		if req.TemplateID == "" {
			return jimeng.VideoSubmitRequest{}, fmt.Errorf("--template is required for %q", preset)
		}
	default:
		return jimeng.VideoSubmitRequest{}, fmt.Errorf(
			"invalid --preset: %q (supported: %q|%q|%q|%q|%q)",
			preset,
			api.VideoPresetT2V720,
			api.VideoPresetT2V1080,
			api.VideoPresetI2VFirst,
			api.VideoPresetI2VFirstTail,
			api.VideoPresetI2VRecamera,
		)
	}

	if err := jimeng.ValidateVideoSubmitRequest(&req); err != nil {
		return jimeng.VideoSubmitRequest{}, err
	}

	return req, nil
}

func parseVideoImageURLs(raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	parts := strings.Split(trimmed, ",")
	urls := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		urls = append(urls, v)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("--image-url must contain at least one non-empty URL")
	}

	return urls, nil
}

func normalizeVideoCameraStrength(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return "", nil
	}

	switch v {
	case string(jimeng.VideoCameraStrengthWeak), string(jimeng.VideoCameraStrengthMedium), string(jimeng.VideoCameraStrengthStrong):
		return v, nil
	default:
		return "", fmt.Errorf("invalid --camera-strength: %q (supported: weak|medium|strong)", raw)
	}
}

func formatVideoSubmitResponse(formatter *output.Formatter, resp *jimeng.VideoSubmitResponse) (string, error) {
	res := videoSubmitResult{}
	if resp != nil {
		res = videoSubmitResult{
			TaskID:      resp.TaskID,
			Preset:      resp.Preset,
			ReqKey:      resp.ReqKey,
			Code:        resp.Code,
			Status:      resp.Status,
			Message:     resp.Message,
			RequestID:   resp.RequestID,
			TimeElapsed: resp.TimeElapsed,
		}
	}

	format := output.FormatText
	if formatter != nil && formatter.Format != "" {
		format = formatter.Format
	}

	switch format {
	case output.FormatJSON:
		b, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	case output.FormatText:
		parts := make([]string, 0, 3)
		parts = append(parts, fmt.Sprintf("TaskID=%s", res.TaskID))
		if res.Preset != "" {
			parts = append(parts, fmt.Sprintf("Preset=%s", res.Preset))
		}
		if res.ReqKey != "" {
			parts = append(parts, fmt.Sprintf("ReqKey=%s", res.ReqKey))
		}
		return strings.Join(parts, " "), nil
	default:
		return "", fmt.Errorf("invalid --format: %q (supported: text|json)", format)
	}
}

var videoQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query video task status",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, formatter, err := newClientAndFormatter(cmd)
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		req, err := buildVideoQueryRequest()
		if err != nil {
			return err
		}

		resp, err := client.GetVideoResult(ctx, req)
		if err != nil {
			return err
		}

		out, err := formatVideoQueryResponse(formatter, req.TaskID, resp)
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), out)
		return nil
	},
}

func buildVideoQueryRequest() (jimeng.VideoGetResultRequest, error) {
	taskID := strings.TrimSpace(videoQueryFlags.taskID)
	if taskID == "" {
		return jimeng.VideoGetResultRequest{}, fmt.Errorf("--task-id is required")
	}

	preset := api.VideoPreset(strings.TrimSpace(videoQueryFlags.preset))
	if preset == "" {
		return jimeng.VideoGetResultRequest{}, fmt.Errorf("--preset is required")
	}

	reqKey, err := api.VideoQueryReqKey(preset)
	if err != nil {
		return jimeng.VideoGetResultRequest{}, fmt.Errorf(
			"invalid --preset: %q (supported: %q|%q|%q|%q|%q)",
			preset,
			api.VideoPresetT2V720,
			api.VideoPresetT2V1080,
			api.VideoPresetI2VFirst,
			api.VideoPresetI2VFirstTail,
			api.VideoPresetI2VRecamera,
		)
	}

	return jimeng.VideoGetResultRequest{
		TaskID: taskID,
		Preset: preset,
		ReqKey: reqKey,
	}, nil
}

func formatVideoQueryResponse(formatter *output.Formatter, taskID string, resp *jimeng.VideoGetResultResponse) (string, error) {
	res := videoQueryResult{TaskID: taskID}
	if resp != nil {
		res.Preset = resp.Preset
		res.ReqKey = resp.ReqKey
		res.Status = string(resp.Status)
		res.VideoURL = resp.VideoURL
		res.Code = resp.Code
		res.Message = resp.Message
		res.RequestID = resp.RequestID
	}

	format := output.FormatText
	if formatter != nil && formatter.Format != "" {
		format = formatter.Format
	}

	switch format {
	case output.FormatJSON:
		b, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	case output.FormatText:
		parts := make([]string, 0, 5)
		parts = append(parts, fmt.Sprintf("TaskID=%s", res.TaskID))
		if res.Status != "" {
			parts = append(parts, fmt.Sprintf("Status=%s", res.Status))
		}
		if res.Preset != "" {
			parts = append(parts, fmt.Sprintf("Preset=%s", res.Preset))
		}
		if res.ReqKey != "" {
			parts = append(parts, fmt.Sprintf("ReqKey=%s", res.ReqKey))
		}
		if res.VideoURL != "" {
			parts = append(parts, fmt.Sprintf("VideoURL=%s", res.VideoURL))
		}
		return strings.Join(parts, " "), nil
	default:
		return "", fmt.Errorf("invalid --format: %q (supported: text|json)", format)
	}
}

var videoWaitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Wait for video task completion",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, formatter, err := newClientAndFormatter(cmd)
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		req, err := buildVideoWaitRequest()
		if err != nil {
			return err
		}

		resp, err := client.VideoWait(ctx, req.TaskID, req.Preset, req.Options)
		if err != nil {
			return err
		}

		out, err := formatVideoWaitResponse(formatter, req.TaskID, req.Preset, resp)
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), out)
		return nil
	},
}

func buildVideoWaitRequest() (videoWaitRequest, error) {
	taskID := strings.TrimSpace(videoWaitFlags.taskID)
	if taskID == "" {
		return videoWaitRequest{}, fmt.Errorf("--task-id is required")
	}

	preset := api.VideoPreset(strings.TrimSpace(videoWaitFlags.preset))
	if preset == "" {
		return videoWaitRequest{}, fmt.Errorf("--preset is required")
	}
	if _, err := api.VideoQueryReqKey(preset); err != nil {
		return videoWaitRequest{}, fmt.Errorf(
			"invalid --preset: %q (supported: %q|%q|%q|%q|%q)",
			preset,
			api.VideoPresetT2V720,
			api.VideoPresetT2V1080,
			api.VideoPresetI2VFirst,
			api.VideoPresetI2VFirstTail,
			api.VideoPresetI2VRecamera,
		)
	}

	interval, err := parseVideoWaitDuration("--interval", videoWaitFlags.interval)
	if err != nil {
		return videoWaitRequest{}, err
	}
	timeout, err := parseVideoWaitDuration("--wait-timeout", videoWaitFlags.timeout)
	if err != nil {
		return videoWaitRequest{}, err
	}

	return videoWaitRequest{
		TaskID: taskID,
		Preset: preset,
		Options: jimeng.WaitOptions{
			Interval: interval,
			Timeout:  timeout,
		},
	}, nil
}

func parseVideoWaitDuration(flagName, raw string) (time.Duration, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, nil
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: must be a duration like 2s or 5m: %w", flagName, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid %s %q: must be greater than 0", flagName, raw)
	}

	return d, nil
}

func formatVideoWaitResponse(formatter *output.Formatter, taskID string, preset api.VideoPreset, resp *jimeng.VideoWaitResult) (string, error) {
	res := videoWaitResult{TaskID: taskID, Preset: preset}
	if resp != nil {
		res.Status = string(resp.Status)
		res.VideoURL = resp.VideoURL
		res.PollCount = resp.PollCount
	}

	format := output.FormatText
	if formatter != nil && formatter.Format != "" {
		format = formatter.Format
	}

	switch format {
	case output.FormatJSON:
		b, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	case output.FormatText:
		parts := make([]string, 0, 5)
		parts = append(parts, fmt.Sprintf("TaskID=%s", res.TaskID))
		if res.Status != "" {
			parts = append(parts, fmt.Sprintf("Status=%s", res.Status))
		}
		if res.Preset != "" {
			parts = append(parts, fmt.Sprintf("Preset=%s", res.Preset))
		}
		if res.VideoURL != "" {
			parts = append(parts, fmt.Sprintf("VideoURL=%s", res.VideoURL))
		}
		if res.PollCount > 0 {
			parts = append(parts, fmt.Sprintf("PollCount=%d", res.PollCount))
		}
		return strings.Join(parts, " "), nil
	default:
		return "", fmt.Errorf("invalid --format: %q (supported: text|json)", format)
	}
}

var videoDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download video task result",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, formatter, err := newClientAndFormatter(cmd)
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		taskID := strings.TrimSpace(videoDownloadFlags.taskID)
		preset := api.VideoPreset(strings.TrimSpace(videoDownloadFlags.preset))

		reqKey, err := api.VideoQueryReqKey(preset)
		if err != nil {
			return err
		}

		resp, err := client.GetVideoResult(ctx, jimeng.VideoGetResultRequest{
			TaskID: taskID,
			Preset: preset,
			ReqKey: reqKey,
		})
		if err != nil {
			return err
		}

		if resp.Status != jimeng.VideoStatusDone {
			return fmt.Errorf("task is not done: status=%s", resp.Status)
		}

		if resp.VideoURL == "" {
			return fmt.Errorf("task is done but video_url is empty")
		}

		filePath, err := client.DownloadVideo(ctx, taskID, resp.VideoURL, jimeng.FlowOptions{
			DownloadDir: videoDownloadFlags.dir,
			Overwrite:   videoDownloadFlags.overwrite,
		})
		if err != nil {
			return err
		}

		res := output.VideoDownloadResult{
			TaskID:   taskID,
			Status:   string(resp.Status),
			VideoURL: resp.VideoURL,
			File:     filePath,
		}

		out, err := formatter.FormatVideoDownloadResult(res)
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), out)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(videoCmd)
	videoCmd.AddCommand(videoSubmitCmd)
	videoCmd.AddCommand(videoQueryCmd)
	videoCmd.AddCommand(videoWaitCmd)
	videoCmd.AddCommand(videoDownloadCmd)

	// Video Submit Flags
	videoSubmitCmd.Flags().StringVar(&videoSubmitFlags.preset, "preset", "", "Model preset (required)")
	videoSubmitCmd.Flags().StringVar(&videoSubmitFlags.prompt, "prompt", "", "Prompt text")
	videoSubmitCmd.Flags().IntVar(&videoSubmitFlags.frames, "frames", 0, "Number of frames")
	videoSubmitCmd.Flags().StringVar(&videoSubmitFlags.aspectRatio, "aspect-ratio", "", "Aspect ratio")
	videoSubmitCmd.Flags().StringVar(&videoSubmitFlags.imageURL, "image-url", "", "Input image URL")
	videoSubmitCmd.Flags().StringVar(&videoSubmitFlags.template, "template", "", "Template ID")
	videoSubmitCmd.Flags().StringVar(&videoSubmitFlags.cameraStrength, "camera-strength", "", "Camera strength: weak|medium|strong")
	_ = videoSubmitCmd.MarkFlagRequired("preset")

	// Video Query Flags
	videoQueryCmd.Flags().StringVar(&videoQueryFlags.taskID, "task-id", "", "Task ID (required)")
	videoQueryCmd.Flags().StringVar(&videoQueryFlags.preset, "preset", "", "Model preset (required)")
	_ = videoQueryCmd.MarkFlagRequired("task-id")
	_ = videoQueryCmd.MarkFlagRequired("preset")

	// Video Wait Flags
	videoWaitCmd.Flags().StringVar(&videoWaitFlags.taskID, "task-id", "", "Task ID (required)")
	videoWaitCmd.Flags().StringVar(&videoWaitFlags.preset, "preset", "", "Model preset (required)")
	videoWaitCmd.Flags().StringVar(&videoWaitFlags.interval, "interval", "2s", "Poll interval duration, e.g. 2s")
	videoWaitCmd.Flags().StringVar(&videoWaitFlags.timeout, "wait-timeout", "5m", "Max wait duration, e.g. 60s, 5m")
	_ = videoWaitCmd.MarkFlagRequired("task-id")
	_ = videoWaitCmd.MarkFlagRequired("preset")

	// Video Download Flags
	videoDownloadCmd.Flags().StringVar(&videoDownloadFlags.taskID, "task-id", "", "Task ID (required)")
	videoDownloadCmd.Flags().StringVar(&videoDownloadFlags.preset, "preset", "", "Model preset (required)")
	videoDownloadCmd.Flags().StringVar(&videoDownloadFlags.dir, "dir", "", "Download directory (required)")
	videoDownloadCmd.Flags().BoolVar(&videoDownloadFlags.overwrite, "overwrite", false, "Overwrite existing files")
	_ = videoDownloadCmd.MarkFlagRequired("task-id")
	_ = videoDownloadCmd.MarkFlagRequired("preset")
	_ = videoDownloadCmd.MarkFlagRequired("dir")
}
