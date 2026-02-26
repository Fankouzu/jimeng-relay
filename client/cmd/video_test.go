package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jimeng-relay/client/internal/api"
	"github.com/jimeng-relay/client/internal/jimeng"
	"github.com/jimeng-relay/client/internal/output"
)

func TestVideoCommandHelp(t *testing.T) {
	rootCmd := RootCmd()
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"video", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	out := b.String()
	subcommands := []string{"submit", "query", "wait", "download"}
	for _, sub := range subcommands {
		if !strings.Contains(out, sub) {
			t.Errorf("expected video help output to contain subcommand %q", sub)
		}
	}
}

func TestVideoSubmitFlags(t *testing.T) {
	rootCmd := RootCmd()
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"video", "submit", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	out := b.String()
	expectedFlags := []string{
		"--preset",
		"--prompt",
		"--frames",
		"--aspect-ratio",
		"--image-url",
		"--image-file",
		"--template",
		"--camera-strength",
	}
	for _, flag := range expectedFlags {
		if !strings.Contains(out, flag) {
			t.Errorf("expected video submit help output to contain flag %q", flag)
		}
	}
}

func TestVideoQueryFlags(t *testing.T) {
	rootCmd := RootCmd()
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"video", "query", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	out := b.String()
	expectedFlags := []string{
		"--task-id",
		"--preset",
	}
	for _, flag := range expectedFlags {
		if !strings.Contains(out, flag) {
			t.Errorf("expected video query help output to contain flag %q", flag)
		}
	}
}

func TestVideoQuery_BuildRequestPresetMapping(t *testing.T) {
	reset := func() {
		videoQueryFlags = videoQueryFlagValues{}
	}

	t.Run("maps preset to req_key", func(t *testing.T) {
		reset()
		videoQueryFlags.taskID = "task-video-99"
		videoQueryFlags.preset = string(api.VideoPresetI2VFirstTail)

		req, err := buildVideoQueryRequest()
		if err != nil {
			t.Fatalf("buildVideoQueryRequest returned error: %v", err)
		}
		if req.TaskID != "task-video-99" {
			t.Fatalf("expected task id task-video-99, got=%q", req.TaskID)
		}
		if req.Preset != api.VideoPresetI2VFirstTail {
			t.Fatalf("expected preset i2v-first-tail, got=%q", req.Preset)
		}
		if req.ReqKey != api.ReqKeyJimengI2VFirstTailV30_1080 {
			t.Fatalf("expected req_key %q, got=%q", api.ReqKeyJimengI2VFirstTailV30_1080, req.ReqKey)
		}
	})

	t.Run("validates missing task-id", func(t *testing.T) {
		reset()
		videoQueryFlags.preset = string(api.VideoPresetT2V720)

		_, err := buildVideoQueryRequest()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "--task-id is required") {
			t.Fatalf("expected task-id validation error, got=%v", err)
		}
	})

	t.Run("validates missing preset", func(t *testing.T) {
		reset()
		videoQueryFlags.taskID = "task-video-99"

		_, err := buildVideoQueryRequest()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "--preset is required") {
			t.Fatalf("expected preset-required validation error, got=%v", err)
		}
	})

	t.Run("validates invalid preset", func(t *testing.T) {
		reset()
		videoQueryFlags.taskID = "task-video-99"
		videoQueryFlags.preset = "invalid"

		_, err := buildVideoQueryRequest()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid --preset") {
			t.Fatalf("expected invalid-preset validation error, got=%v", err)
		}
	})
}

func TestVideoQuery_FormatResponseIncludesStatusPresetReqKeyAndVideoURL(t *testing.T) {
	res, err := formatVideoQueryResponse(output.NewFormatter(output.FormatText), "task-video-42", &jimeng.VideoGetResultResponse{
		Preset:   api.VideoPresetT2V720,
		ReqKey:   api.ReqKeyJimengT2VV30_720p,
		Status:   jimeng.VideoStatusDone,
		VideoURL: "https://cdn.example.com/video.mp4",
	})
	if err != nil {
		t.Fatalf("formatVideoQueryResponse returned error: %v", err)
	}
	if !strings.Contains(res, "TaskID=task-video-42") {
		t.Fatalf("expected TaskID in text output, got=%q", res)
	}
	if !strings.Contains(res, "Status=done") {
		t.Fatalf("expected status in text output, got=%q", res)
	}
	if !strings.Contains(res, "Preset=t2v-720") {
		t.Fatalf("expected preset in text output, got=%q", res)
	}
	if !strings.Contains(res, "ReqKey=jimeng_t2v_v30_720p") {
		t.Fatalf("expected req_key in text output, got=%q", res)
	}
	if !strings.Contains(res, "VideoURL=https://cdn.example.com/video.mp4") {
		t.Fatalf("expected video url in text output, got=%q", res)
	}
}

func TestVideoWaitFlags(t *testing.T) {
	rootCmd := RootCmd()
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"video", "wait", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	out := b.String()
	expectedFlags := []string{
		"--task-id",
		"--preset",
		"--interval",
		"--wait-timeout",
	}
	for _, flag := range expectedFlags {
		if !strings.Contains(out, flag) {
			t.Errorf("expected video wait help output to contain flag %q", flag)
		}
	}
}

func TestVideoWait_BuildRequest(t *testing.T) {
	reset := func() {
		videoWaitFlags = videoWaitFlagValues{}
	}

	t.Run("parses required fields and durations", func(t *testing.T) {
		reset()
		videoWaitFlags.taskID = "task-video-42"
		videoWaitFlags.preset = string(api.VideoPresetT2V720)
		videoWaitFlags.interval = "3s"
		videoWaitFlags.timeout = "90s"

		req, err := buildVideoWaitRequest()
		if err != nil {
			t.Fatalf("buildVideoWaitRequest returned error: %v", err)
		}
		if req.TaskID != "task-video-42" {
			t.Fatalf("expected task id task-video-42, got=%q", req.TaskID)
		}
		if req.Preset != api.VideoPresetT2V720 {
			t.Fatalf("expected preset t2v-720, got=%q", req.Preset)
		}
		if req.Options.Interval != 3*time.Second {
			t.Fatalf("expected interval=3s, got=%s", req.Options.Interval)
		}
		if req.Options.Timeout != 90*time.Second {
			t.Fatalf("expected timeout=90s, got=%s", req.Options.Timeout)
		}
	})

	t.Run("validates missing task-id", func(t *testing.T) {
		reset()
		videoWaitFlags.preset = string(api.VideoPresetT2V720)

		_, err := buildVideoWaitRequest()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "--task-id is required") {
			t.Fatalf("expected task-id validation error, got=%v", err)
		}
	})

	t.Run("validates missing preset", func(t *testing.T) {
		reset()
		videoWaitFlags.taskID = "task-video-42"

		_, err := buildVideoWaitRequest()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "--preset is required") {
			t.Fatalf("expected preset validation error, got=%v", err)
		}
	})

	t.Run("validates invalid preset", func(t *testing.T) {
		reset()
		videoWaitFlags.taskID = "task-video-42"
		videoWaitFlags.preset = "invalid"

		_, err := buildVideoWaitRequest()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid --preset") {
			t.Fatalf("expected invalid-preset error, got=%v", err)
		}
	})

	t.Run("validates interval duration", func(t *testing.T) {
		reset()
		videoWaitFlags.taskID = "task-video-42"
		videoWaitFlags.preset = string(api.VideoPresetT2V720)
		videoWaitFlags.interval = "not-a-duration"

		_, err := buildVideoWaitRequest()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid --interval") {
			t.Fatalf("expected interval validation error, got=%v", err)
		}
	})

	t.Run("validates timeout duration", func(t *testing.T) {
		reset()
		videoWaitFlags.taskID = "task-video-42"
		videoWaitFlags.preset = string(api.VideoPresetT2V720)
		videoWaitFlags.timeout = "0s"

		_, err := buildVideoWaitRequest()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid --wait-timeout") {
			t.Fatalf("expected timeout validation error, got=%v", err)
		}
	})
}

func TestVideoWait_FormatResponseIncludesStatusPresetVideoURLAndPollCount(t *testing.T) {
	res, err := formatVideoWaitResponse(output.NewFormatter(output.FormatText), "task-video-42", api.VideoPresetT2V720, &jimeng.VideoWaitResult{
		Status:    jimeng.VideoStatusDone,
		VideoURL:  "https://cdn.example.com/video.mp4",
		PollCount: 3,
	})
	if err != nil {
		t.Fatalf("formatVideoWaitResponse returned error: %v", err)
	}
	if !strings.Contains(res, "TaskID=task-video-42") {
		t.Fatalf("expected TaskID in text output, got=%q", res)
	}
	if !strings.Contains(res, "Status=done") {
		t.Fatalf("expected status in text output, got=%q", res)
	}
	if !strings.Contains(res, "Preset=t2v-720") {
		t.Fatalf("expected preset in text output, got=%q", res)
	}
	if !strings.Contains(res, "VideoURL=https://cdn.example.com/video.mp4") {
		t.Fatalf("expected video url in text output, got=%q", res)
	}
	if !strings.Contains(res, "PollCount=3") {
		t.Fatalf("expected poll count in text output, got=%q", res)
	}
}

func TestVideoWait_FormatResponseJSON(t *testing.T) {
	res, err := formatVideoWaitResponse(output.NewFormatter(output.FormatJSON), "task-video-99", api.VideoPresetI2VFirst, &jimeng.VideoWaitResult{
		Status:    jimeng.VideoStatusDone,
		VideoURL:  "https://cdn.example.com/video-99.mp4",
		PollCount: 2,
	})
	if err != nil {
		t.Fatalf("formatVideoWaitResponse returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(res), &parsed); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}
	if parsed["task_id"] != "task-video-99" {
		t.Fatalf("expected task_id task-video-99, got=%v", parsed["task_id"])
	}
	if parsed["preset"] != "i2v-first" {
		t.Fatalf("expected preset i2v-first, got=%v", parsed["preset"])
	}
	if parsed["status"] != "done" {
		t.Fatalf("expected status done, got=%v", parsed["status"])
	}
}

func TestVideoDownloadFlags(t *testing.T) {
	rootCmd := RootCmd()
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"video", "download", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	out := b.String()
	expectedFlags := []string{
		"--task-id",
		"--preset",
		"--dir",
		"--overwrite",
	}
	for _, flag := range expectedFlags {
		if !strings.Contains(out, flag) {
			t.Errorf("expected video download help output to contain flag %q", flag)
		}
	}
}

func TestVideoSubmit_BuildRequestPresetMapping(t *testing.T) {
	reset := func() {
		videoSubmitFlags = videoSubmitFlagValues{}
	}

	t.Run("t2v defaults", func(t *testing.T) {
		reset()
		videoSubmitFlags.preset = string(api.VideoPresetT2V720)
		videoSubmitFlags.prompt = "a cinematic sunrise"

		req, err := buildVideoSubmitRequest()
		if err != nil {
			t.Fatalf("buildVideoSubmitRequest returned error: %v", err)
		}

		if req.Frames != 121 {
			t.Fatalf("expected default frames=121, got=%d", req.Frames)
		}
		if req.AspectRatio != "16:9" {
			t.Fatalf("expected default aspect_ratio=16:9, got=%q", req.AspectRatio)
		}
	})

	t.Run("i2v-first-tail parses two image urls", func(t *testing.T) {
		reset()
		videoSubmitFlags.preset = string(api.VideoPresetI2VFirstTail)
		videoSubmitFlags.prompt = "animate transition"
		videoSubmitFlags.imageURL = "https://example.com/first.png, https://example.com/tail.png"

		req, err := buildVideoSubmitRequest()
		if err != nil {
			t.Fatalf("buildVideoSubmitRequest returned error: %v", err)
		}

		if len(req.ImageURLs) != 2 {
			t.Fatalf("expected 2 image urls, got=%d", len(req.ImageURLs))
		}
	})

	t.Run("recamera requires template and normalizes camera strength", func(t *testing.T) {
		reset()
		videoSubmitFlags.preset = string(api.VideoPresetI2VRecamera)
		videoSubmitFlags.prompt = "orbit the subject"
		videoSubmitFlags.imageURL = "https://example.com/frame.png"
		videoSubmitFlags.template = "dynamic_orbit"
		videoSubmitFlags.cameraStrength = "STRONG"

		req, err := buildVideoSubmitRequest()
		if err != nil {
			t.Fatalf("buildVideoSubmitRequest returned error: %v", err)
		}

		if req.CameraStrength != "strong" {
			t.Fatalf("expected normalized camera_strength=strong, got=%q", req.CameraStrength)
		}
	})

	t.Run("i2v-first-tail validates image count", func(t *testing.T) {
		reset()
		videoSubmitFlags.preset = string(api.VideoPresetI2VFirstTail)
		videoSubmitFlags.prompt = "animate transition"
		videoSubmitFlags.imageURL = "https://example.com/first.png"

		_, err := buildVideoSubmitRequest()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "requires exactly 2 URLs") {
			t.Fatalf("expected image-count validation error, got=%v", err)
		}
	})
}

func TestVideoSubmit_FormatResponseIncludesPresetAndReqKey(t *testing.T) {
	res, err := formatVideoSubmitResponse(output.NewFormatter(output.FormatText), &jimeng.VideoSubmitResponse{
		TaskID: "task-video-42",
		Preset: api.VideoPresetT2V720,
		ReqKey: api.ReqKeyJimengT2VV30_720p,
	})
	if err != nil {
		t.Fatalf("formatVideoSubmitResponse returned error: %v", err)
	}
	if !strings.Contains(res, "TaskID=task-video-42") {
		t.Fatalf("expected TaskID in text output, got=%q", res)
	}
	if !strings.Contains(res, "Preset=t2v-720") {
		t.Fatalf("expected preset in text output, got=%q", res)
	}
	if !strings.Contains(res, "ReqKey=jimeng_t2v_v30_720p") {
		t.Fatalf("expected req_key in text output, got=%q", res)
	}
}

func TestVideoSubmit_ImageFileContract(t *testing.T) {
	// This test covers the CLI contract for --image-file which is not yet implemented.
	// These tests are expected to FAIL (RED phase).

	t.Run("accepts single --image-file for i2v-first", func(t *testing.T) {
		rootCmd := RootCmd()
		rootCmd.SetArgs([]string{"video", "submit", "--preset", "i2v-first", "--prompt", "test", "--image-file", "nonexistent.png"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
	})

	t.Run("accepts two --image-file for i2v-first-tail", func(t *testing.T) {
		rootCmd := RootCmd()
		rootCmd.SetArgs([]string{"video", "submit", "--preset", "i2v-first-tail", "--prompt", "test", "--image-file", "f1.png", "--image-file", "f2.png"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
	})

	t.Run("rejects wrong count for i2v-first-tail", func(t *testing.T) {
		rootCmd := RootCmd()
		rootCmd.SetArgs([]string{"video", "submit", "--preset", "i2v-first-tail", "--prompt", "test", "--image-file", "f1.png"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for wrong image count, got nil")
		}
		if !strings.Contains(err.Error(), "requires exactly 2") {
			t.Fatalf("expected count mismatch error, got: %v", err)
		}
	})

	t.Run("rejects --image-file for t2v presets", func(t *testing.T) {
		rootCmd := RootCmd()
		rootCmd.SetArgs([]string{"video", "submit", "--preset", "t2v-720", "--prompt", "test", "--image-file", "f1.png"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for --image-file with t2v preset, got nil")
		}
		if !strings.Contains(err.Error(), "not allowed for") {
			t.Fatalf("expected not-allowed error, got: %v", err)
		}
	})

	t.Run("mutual exclusivity with --image-url", func(t *testing.T) {
		rootCmd := RootCmd()
		rootCmd.SetArgs([]string{"video", "submit", "--preset", "i2v-first", "--prompt", "test", "--image-file", "f1.png", "--image-url", "http://example.com/i.png"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for both --image-file and --image-url, got nil")
		}
		if !strings.Contains(err.Error(), "cannot be used together") {
			t.Fatalf("expected mutual exclusivity error, got: %v", err)
		}
	})
}
