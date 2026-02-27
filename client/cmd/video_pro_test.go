package cmd

import (
	"testing"

	"github.com/jimeng-relay/client/internal/api"
)

func TestVideoSubmit_ProPresets(t *testing.T) {
	reset := func() {
		videoSubmitFlags = videoSubmitFlagValues{}
	}

	t.Run("t2v-pro mapping", func(t *testing.T) {
		reset()
		videoSubmitFlags.preset = string(api.VideoPresetT2VPro)
		videoSubmitFlags.prompt = "a cinematic pro video"

		req, err := buildVideoSubmitRequest()
		if err != nil {
			t.Fatalf("buildVideoSubmitRequest returned error: %v", err)
		}

		if req.Preset != api.VideoPresetT2VPro {
			t.Fatalf("expected preset %q, got=%q", api.VideoPresetT2VPro, req.Preset)
		}
	})

	t.Run("i2v-first-pro mapping", func(t *testing.T) {
		reset()
		videoSubmitFlags.preset = string(api.VideoPresetI2VFirstPro)
		videoSubmitFlags.prompt = "animate pro video"
		videoSubmitFlags.imageURL = "https://example.com/pro.png"

		req, err := buildVideoSubmitRequest()
		if err != nil {
			t.Fatalf("buildVideoSubmitRequest returned error: %v", err)
		}

		if req.Preset != api.VideoPresetI2VFirstPro {
			t.Fatalf("expected preset %q, got=%q", api.VideoPresetI2VFirstPro, req.Preset)
		}
		if len(req.ImageURLs) != 1 {
			t.Fatalf("expected 1 image url, got=%d", len(req.ImageURLs))
		}
	})

	t.Run("t2v-pro rejects image", func(t *testing.T) {
		reset()
		videoSubmitFlags.preset = string(api.VideoPresetT2VPro)
		videoSubmitFlags.prompt = "test"
		videoSubmitFlags.imageURL = "https://example.com/img.png"

		_, err := buildVideoSubmitRequest()
		if err == nil {
			t.Fatal("expected error for t2v-pro with image, got nil")
		}
	})

	t.Run("i2v-first-pro requires image", func(t *testing.T) {
		reset()
		videoSubmitFlags.preset = string(api.VideoPresetI2VFirstPro)
		videoSubmitFlags.prompt = "test"

		_, err := buildVideoSubmitRequest()
		if err == nil {
			t.Fatal("expected error for i2v-first-pro without image, got nil")
		}
	})
}

func TestVideoQuery_ProPresets(t *testing.T) {
	reset := func() {
		videoQueryFlags = videoQueryFlagValues{}
	}

	t.Run("t2v-pro query mapping", func(t *testing.T) {
		reset()
		videoQueryFlags.taskID = "task-pro-1"
		videoQueryFlags.preset = string(api.VideoPresetT2VPro)

		req, err := buildVideoQueryRequest()
		if err != nil {
			t.Fatalf("buildVideoQueryRequest returned error: %v", err)
		}

		if req.ReqKey != api.ReqKeyJimengT2VV30Pro {
			t.Fatalf("expected req_key %q, got=%q", api.ReqKeyJimengT2VV30Pro, req.ReqKey)
		}
	})

	t.Run("i2v-first-pro query mapping", func(t *testing.T) {
		reset()
		videoQueryFlags.taskID = "task-pro-2"
		videoQueryFlags.preset = string(api.VideoPresetI2VFirstPro)

		req, err := buildVideoQueryRequest()
		if err != nil {
			t.Fatalf("buildVideoQueryRequest returned error: %v", err)
		}

		if req.ReqKey != api.ReqKeyJimengI2VFirstV30Pro {
			t.Fatalf("expected req_key %q, got=%q", api.ReqKeyJimengI2VFirstV30Pro, req.ReqKey)
		}
	})
}
