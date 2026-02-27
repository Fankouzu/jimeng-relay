package cmd

import (
	"encoding/json"
	"testing"

	"github.com/jimeng-relay/client/internal/api"
	"github.com/jimeng-relay/client/internal/jimeng"
	"github.com/jimeng-relay/client/internal/output"
)

func TestLegacyVideoSubmitOutputFormat(t *testing.T) {
	resp := &jimeng.VideoSubmitResponse{
		TaskID:      "task-video-123",
		Preset:      api.VideoPresetT2V720,
		ReqKey:      api.ReqKeyJimengT2VV30_720p,
		Code:        10000,
		Status:      10000,
		Message:     "success",
		RequestID:   "req-456",
		TimeElapsed: 100,
	}

	t.Run("TextFormat", func(t *testing.T) {
		formatter := output.NewFormatter(output.FormatText)
		out, err := formatVideoSubmitResponse(formatter, resp)
		if err != nil {
			t.Fatalf("formatVideoSubmitResponse failed: %v", err)
		}

		expected := "TaskID=task-video-123 Preset=t2v-720 ReqKey=jimeng_t2v_v30"
		if out != expected {
			t.Errorf("expected %q, got %q", expected, out)
		}
	})

	t.Run("JSONFormat", func(t *testing.T) {
		formatter := output.NewFormatter(output.FormatJSON)
		out, err := formatVideoSubmitResponse(formatter, resp)
		if err != nil {
			t.Fatalf("formatVideoSubmitResponse failed: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		assertions := map[string]interface{}{
			"task_id":      "task-video-123",
			"preset":       "t2v-720",
			"req_key":      "jimeng_t2v_v30",
			"code":         float64(10000),
			"status":       float64(10000),
			"message":      "success",
			"request_id":   "req-456",
			"time_elapsed": float64(100),
		}

		for k, v := range assertions {
			if parsed[k] != v {
				t.Errorf("expected field %q to be %v, got %v", k, v, parsed[k])
			}
		}
	})
}

func TestLegacyVideoQueryOutputFormat(t *testing.T) {
	resp := &jimeng.VideoGetResultResponse{
		Preset:    api.VideoPresetT2V720,
		ReqKey:    api.ReqKeyJimengT2VV30_720p,
		Status:    jimeng.VideoStatusDone,
		VideoURL:  "https://example.com/video.mp4",
		Code:      10000,
		Message:   "success",
		RequestID: "req-789",
	}

	t.Run("TextFormat", func(t *testing.T) {
		formatter := output.NewFormatter(output.FormatText)
		out, err := formatVideoQueryResponse(formatter, "task-video-123", resp)
		if err != nil {
			t.Fatalf("formatVideoQueryResponse failed: %v", err)
		}

		expected := "TaskID=task-video-123 Status=done Preset=t2v-720 ReqKey=jimeng_t2v_v30 VideoURL=https://example.com/video.mp4"
		if out != expected {
			t.Errorf("expected %q, got %q", expected, out)
		}
	})

	t.Run("JSONFormat", func(t *testing.T) {
		formatter := output.NewFormatter(output.FormatJSON)
		out, err := formatVideoQueryResponse(formatter, "task-video-123", resp)
		if err != nil {
			t.Fatalf("formatVideoQueryResponse failed: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		assertions := map[string]interface{}{
			"task_id":    "task-video-123",
			"preset":     "t2v-720",
			"req_key":    "jimeng_t2v_v30",
			"status":     "done",
			"video_url":  "https://example.com/video.mp4",
			"code":       float64(10000),
			"message":    "success",
			"request_id": "req-789",
		}

		for k, v := range assertions {
			if parsed[k] != v {
				t.Errorf("expected field %q to be %v, got %v", k, v, parsed[k])
			}
		}
	})
}

func TestLegacyVideoWaitOutputFormat(t *testing.T) {
	resp := &jimeng.VideoWaitResult{
		Status:    jimeng.VideoStatusDone,
		VideoURL:  "https://example.com/video.mp4",
		PollCount: 5,
	}

	t.Run("TextFormat", func(t *testing.T) {
		formatter := output.NewFormatter(output.FormatText)
		out, err := formatVideoWaitResponse(formatter, "task-video-123", api.VideoPresetT2V720, resp)
		if err != nil {
			t.Fatalf("formatVideoWaitResponse failed: %v", err)
		}

		expected := "TaskID=task-video-123 Status=done Preset=t2v-720 VideoURL=https://example.com/video.mp4 PollCount=5"
		if out != expected {
			t.Errorf("expected %q, got %q", expected, out)
		}
	})

	t.Run("JSONFormat", func(t *testing.T) {
		formatter := output.NewFormatter(output.FormatJSON)
		out, err := formatVideoWaitResponse(formatter, "task-video-123", api.VideoPresetT2V720, resp)
		if err != nil {
			t.Fatalf("formatVideoWaitResponse failed: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		assertions := map[string]interface{}{
			"task_id":    "task-video-123",
			"preset":     "t2v-720",
			"status":     "done",
			"video_url":  "https://example.com/video.mp4",
			"poll_count": float64(5),
		}

		for k, v := range assertions {
			if parsed[k] != v {
				t.Errorf("expected field %q to be %v, got %v", k, v, parsed[k])
			}
		}
	})
}

func TestLegacyVideoDownloadOutputFormat(t *testing.T) {
	res := output.VideoDownloadResult{
		TaskID:   "task-video-123",
		Status:   "done",
		VideoURL: "https://example.com/video.mp4",
		File:     "/tmp/task-video-123-video.mp4",
	}

	t.Run("TextFormat", func(t *testing.T) {
		formatter := output.NewFormatter(output.FormatText)
		out, err := formatter.FormatVideoDownloadResult(res)
		if err != nil {
			t.Fatalf("FormatVideoDownloadResult failed: %v", err)
		}

		expected := "TaskID=task-video-123 Status=done File=/tmp/task-video-123-video.mp4"
		if out != expected {
			t.Errorf("expected %q, got %q", expected, out)
		}
	})

	t.Run("JSONFormat", func(t *testing.T) {
		formatter := output.NewFormatter(output.FormatJSON)
		out, err := formatter.FormatVideoDownloadResult(res)
		if err != nil {
			t.Fatalf("FormatVideoDownloadResult failed: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		assertions := map[string]interface{}{
			"task_id":   "task-video-123",
			"status":    "done",
			"video_url": "https://example.com/video.mp4",
			"file":      "/tmp/task-video-123-video.mp4",
		}

		for k, v := range assertions {
			if parsed[k] != v {
				t.Errorf("expected field %q to be %v, got %v", k, v, parsed[k])
			}
		}
	})
}
