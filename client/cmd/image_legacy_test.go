package cmd

import (
	"encoding/json"
	"testing"

	"github.com/jimeng-relay/client/internal/jimeng"
	"github.com/jimeng-relay/client/internal/output"
)

func TestLegacyImageSubmitOutputFormat(t *testing.T) {
	resp := &jimeng.SubmitResponse{
		TaskID:      "task-image-123",
		Code:        10000,
		Status:      10000,
		Message:     "success",
		RequestID:   "req-456",
		TimeElapsed: 100,
	}

	t.Run("TextFormat", func(t *testing.T) {
		formatter := output.NewFormatter(output.FormatText)
		out, err := formatter.FormatSubmitResponse(resp)
		if err != nil {
			t.Fatalf("FormatSubmitResponse failed: %v", err)
		}

		expected := "TaskID=task-image-123"
		if out != expected {
			t.Errorf("expected %q, got %q", expected, out)
		}
	})

	t.Run("JSONFormat", func(t *testing.T) {
		formatter := output.NewFormatter(output.FormatJSON)
		out, err := formatter.FormatSubmitResponse(resp)
		if err != nil {
			t.Fatalf("FormatSubmitResponse failed: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		assertions := map[string]interface{}{
			"task_id":      "task-image-123",
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

func TestLegacyImageQueryOutputFormat(t *testing.T) {
	resp := &jimeng.GetResultResponse{
		Status:    jimeng.StatusDone,
		ImageURLs: []string{"https://example.com/image1.png", "https://example.com/image2.png"},
	}

	t.Run("TextFormat", func(t *testing.T) {
		formatter := output.NewFormatter(output.FormatText)
		out, err := formatter.FormatGetResultResponse(resp)
		if err != nil {
			t.Fatalf("FormatGetResultResponse failed: %v", err)
		}

		expected := "Status=done ImageURLs=https://example.com/image1.png,https://example.com/image2.png"
		if out != expected {
			t.Errorf("expected %q, got %q", expected, out)
		}
	})

	t.Run("JSONFormat", func(t *testing.T) {
		formatter := output.NewFormatter(output.FormatJSON)
		out, err := formatter.FormatGetResultResponse(resp)
		if err != nil {
			t.Fatalf("FormatGetResultResponse failed: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		if parsed["Status"] != "done" {
			t.Errorf("expected Status done, got %v", parsed["Status"])
		}

		urls := parsed["ImageURLs"].([]interface{})
		if len(urls) != 2 || urls[0] != "https://example.com/image1.png" || urls[1] != "https://example.com/image2.png" {
			t.Errorf("unexpected ImageURLs: %v", urls)
		}
	})
}

func TestLegacyImageWaitOutputFormat(t *testing.T) {
	// Image wait/download flow uses formatFlowResult in submit.go
	// Since formatFlowResult is not exported, we can't test it directly from another package,
	// but here we are in package 'cmd', so we can.

	res := &jimeng.FlowResult{
		TaskID:     "task-image-123",
		Status:     jimeng.StatusDone,
		ImageURLs:  []string{"https://example.com/image1.png"},
		LocalFiles: []string{"/tmp/task-image-123-image-1.png"},
	}

	t.Run("TextFormat", func(t *testing.T) {
		// formatFlowResult uses rootFlags.format, so we need to set it or mock it.
		// Actually, formatFlowResult is called with a specific format in mind.
		// Let's check how it's used in submit.go.

		// In submit.go:
		// out, err := formatFlowResult(res)

		// It uses rootFlags.format.
		oldFormat := rootFlags.format
		defer func() { rootFlags.format = oldFormat }()

		rootFlags.format = "text"
		out, err := formatFlowResult(res)
		if err != nil {
			t.Fatalf("formatFlowResult failed: %v", err)
		}

		expected := "TaskID=task-image-123 Status=done ImageURLs=https://example.com/image1.png LocalFiles=/tmp/task-image-123-image-1.png"
		if out != expected {
			t.Errorf("expected %q, got %q", expected, out)
		}
	})

	t.Run("JSONFormat", func(t *testing.T) {
		oldFormat := rootFlags.format
		defer func() { rootFlags.format = oldFormat }()

		rootFlags.format = "json"
		out, err := formatFlowResult(res)
		if err != nil {
			t.Fatalf("formatFlowResult failed: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		if parsed["TaskID"] != "task-image-123" {
			t.Errorf("expected TaskID task-image-123, got %v", parsed["TaskID"])
		}
		if parsed["Status"] != "done" {
			t.Errorf("expected Status done, got %v", parsed["Status"])
		}
	})
}
