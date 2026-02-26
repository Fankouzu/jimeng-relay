package jimeng

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jimeng-relay/client/internal/api"
	"github.com/jimeng-relay/client/internal/config"
	internalerrors "github.com/jimeng-relay/client/internal/errors"
)

func TestVideoValidation_T2VAcceptsLegalFramesAndAspectRatio(t *testing.T) {
	t.Parallel()

	for _, frames := range []int{121, 241} {
		req := &VideoSubmitRequest{
			Variant:     VideoVariantT2V,
			Prompt:      "a cinematic mountain sunrise",
			Frames:      frames,
			AspectRatio: "16:9",
			Seed:        42,
		}

		err := ValidateVideoSubmitRequest(req)
		require.NoError(t, err)
	}
}

func TestVideoValidation_T2VRejectsInvalidFrames(t *testing.T) {
	t.Parallel()

	req := &VideoSubmitRequest{
		Variant:     VideoVariantT2V,
		Prompt:      "a city timelapse",
		Frames:      120,
		AspectRatio: "16:9",
	}

	err := ValidateVideoSubmitRequest(req)
	require.Error(t, err)
	require.ErrorContains(t, err, "frames must be 121 or 241")
}

func TestVideoValidation_T2VRejectsInvalidAspectRatio(t *testing.T) {
	t.Parallel()

	req := &VideoSubmitRequest{
		Variant:     VideoVariantT2V,
		Prompt:      "a city timelapse",
		Frames:      121,
		AspectRatio: "2:1",
	}

	err := ValidateVideoSubmitRequest(req)
	require.Error(t, err)
	require.ErrorContains(t, err, "aspect_ratio is invalid")
}

func TestVideoValidation_I2VFirstTailRequiresTwoImages(t *testing.T) {
	t.Parallel()

	req := &VideoSubmitRequest{
		Variant:   VideoVariantI2VFirstTail,
		Prompt:    "keep identity and animate",
		ImageURLs: []string{"https://example.com/only-one.png"},
	}

	err := ValidateVideoSubmitRequest(req)
	require.Error(t, err)
	require.ErrorContains(t, err, "requires exactly 2 images")
}

func TestVideoValidation_I2VFirstFrameRequiresOneImage(t *testing.T) {
	t.Parallel()

	req := &VideoSubmitRequest{
		Variant:   VideoVariantI2VFirstFrame,
		Prompt:    "keep identity and animate",
		ImageURLs: []string{"https://example.com/first.png", "https://example.com/tail.png"},
	}

	err := ValidateVideoSubmitRequest(req)
	require.Error(t, err)
	require.ErrorContains(t, err, "requires exactly 1 image")
}

func TestVideoValidation_I2VFirstFrameAcceptsSingleImage(t *testing.T) {
	t.Parallel()

	req := &VideoSubmitRequest{
		Variant:   VideoVariantI2VFirstFrame,
		Prompt:    "keep identity and animate",
		ImageURLs: []string{"https://example.com/first.png"},
	}

	err := ValidateVideoSubmitRequest(req)
	require.NoError(t, err)
}

func TestVideoValidation_RecameraRequiresTemplateID(t *testing.T) {
	t.Parallel()

	req := &VideoSubmitRequest{
		Variant:        VideoVariantRecamera,
		Prompt:         "dolly in over subject",
		ImageURLs:      []string{"https://example.com/frame.png"},
		CameraStrength: "medium",
	}

	err := ValidateVideoSubmitRequest(req)
	require.Error(t, err)
	require.ErrorContains(t, err, "template_id is required")
}

func TestVideoValidation_RecameraAcceptsTemplateAndStrength(t *testing.T) {
	t.Parallel()

	req := &VideoSubmitRequest{
		Variant:        VideoVariantRecamera,
		Prompt:         "dynamic orbit around subject",
		ImageURLs:      []string{"https://example.com/frame.png"},
		TemplateID:     "dynamic_orbit",
		CameraStrength: "strong",
	}

	err := ValidateVideoSubmitRequest(req)
	require.NoError(t, err)
}

func TestVideoValidation_RecameraRejectsInvalidCameraStrength(t *testing.T) {
	t.Parallel()

	req := &VideoSubmitRequest{
		Variant:        VideoVariantRecamera,
		Prompt:         "dynamic orbit around subject",
		ImageURLs:      []string{"https://example.com/frame.png"},
		TemplateID:     "dynamic_orbit",
		CameraStrength: "ultra",
	}

	err := ValidateVideoSubmitRequest(req)
	require.Error(t, err)
	require.ErrorContains(t, err, "camera_strength is invalid")
}

func TestVideoValidation_PromptIsRequired(t *testing.T) {
	t.Parallel()

	req := &VideoSubmitRequest{
		Variant: VideoVariantT2V,
		Frames:  121,
	}

	err := ValidateVideoSubmitRequest(req)
	require.Error(t, err)
	require.ErrorContains(t, err, "prompt is required")
}

func TestVideoValidation_StatusConstantsMatchAPI(t *testing.T) {
	t.Parallel()

	require.Equal(t, api.StatusInQueue, string(VideoStatusInQueue))
	require.Equal(t, api.StatusGenerating, string(VideoStatusGenerating))
	require.Equal(t, api.StatusDone, string(VideoStatusDone))
	require.Equal(t, api.StatusNotFound, string(VideoStatusNotFound))
	require.Equal(t, api.StatusExpired, string(VideoStatusExpired))
	require.Equal(t, api.StatusFailed, string(VideoStatusFailed))
}

func TestVideoSubmitQuery_UsesPresetReqKeyAndParsesVideoResult(t *testing.T) {
	t.Parallel()

	type capturedRequest struct {
		Action string
		Body   map[string]interface{}
	}

	calls := make([]capturedRequest, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)

		action := r.URL.Query().Get("Action")
		defer r.Body.Close()

		var body map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)

		calls = append(calls, capturedRequest{Action: action, Body: body})

		switch action {
		case api.ActionSubmitTask:
			_, err = w.Write([]byte(`{"code":10000,"status":10000,"message":"success","request_id":"rid-submit","time_elapsed":12,"data":{"task_id":"task-video-1"}}`))
			require.NoError(t, err)
		case api.ActionGetResult:
			_, err = w.Write([]byte(`{"code":10000,"status":10000,"message":"success","request_id":"rid-query","data":{"status":"done","video_url":"https://example.com/video.mp4"}}`))
			require.NoError(t, err)
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, err = w.Write([]byte(`{"code":400,"status":400,"message":"unknown action"}`))
			require.NoError(t, err)
		}
	}))
	defer server.Close()

	parsedURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	c, err := NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      config.DefaultRegion,
		Host:        parsedURL.Host,
		Scheme:      parsedURL.Scheme,
		Timeout:     3 * time.Second,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	submitResp, err := c.SubmitVideoTask(ctx, VideoSubmitRequest{
		Preset:      api.VideoPresetT2V720,
		Prompt:      "a cinematic mountain sunrise",
		Frames:      121,
		AspectRatio: "16:9",
		Seed:        42,
	})
	require.NoError(t, err)
	require.Equal(t, "task-video-1", submitResp.TaskID)
	require.Equal(t, api.ReqKeyJimengT2VV30_720p, submitResp.ReqKey)

	queryResp, err := c.GetVideoResult(ctx, VideoGetResultRequest{
		TaskID: submitResp.TaskID,
		Preset: submitResp.Preset,
		ReqKey: submitResp.ReqKey,
	})
	require.NoError(t, err)
	require.Equal(t, VideoStatusDone, queryResp.Status)
	require.Equal(t, "https://example.com/video.mp4", queryResp.VideoURL)

	require.Len(t, calls, 2)
	require.Equal(t, api.ActionSubmitTask, calls[0].Action)
	require.Equal(t, api.ReqKeyJimengT2VV30_720p, calls[0].Body["req_key"])
	require.Equal(t, "a cinematic mountain sunrise", calls[0].Body["prompt"])
	require.Equal(t, float64(121), calls[0].Body["frames"])
	require.Equal(t, "16:9", calls[0].Body["aspect_ratio"])

	require.Equal(t, api.ActionGetResult, calls[1].Action)
	require.Equal(t, api.ReqKeyJimengT2VV30_720p, calls[1].Body["req_key"])
	require.Equal(t, "task-video-1", calls[1].Body["task_id"])
}

func TestVideoSubmitQuery_RejectsReqKeyMismatch(t *testing.T) {
	t.Parallel()

	c := &Client{}

	_, err := c.GetVideoResult(context.Background(), VideoGetResultRequest{
		TaskID: "task-video-1",
		Preset: api.VideoPresetI2VFirst,
		ReqKey: api.ReqKeyJimengT2VV30_1080p,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "req_key mismatch")

	_, err = c.SubmitVideoTask(context.Background(), VideoSubmitRequest{
		Preset:    api.VideoPresetI2VFirst,
		ReqKey:    api.ReqKeyJimengT2VV30_1080p,
		Prompt:    "animate this frame",
		ImageURLs: []string{"https://example.com/first.png"},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "req_key mismatch")
}

func TestVideoWait_HappyPath(t *testing.T) {
	t.Parallel()

	sequence := []*VideoGetResultResponse{
		{Status: VideoStatusInQueue},
		{Status: VideoStatusGenerating},
		{Status: VideoStatusDone, VideoURL: "https://example.com/final.mp4"},
	}

	calls := 0
	seenPresets := make([]api.VideoPreset, 0, len(sequence))
	c := &Client{
		videoGetResult: func(_ context.Context, req VideoGetResultRequest) (*VideoGetResultResponse, error) {
			require.Equal(t, "task-123", req.TaskID)
			seenPresets = append(seenPresets, req.Preset)
			idx := calls
			if idx >= len(sequence) {
				idx = len(sequence) - 1
			}
			calls++
			return sequence[idx], nil
		},
	}

	resp, err := c.VideoWait(context.Background(), "task-123", api.VideoPresetT2V720, WaitOptions{
		Interval: time.Millisecond,
		Timeout:  time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, VideoStatusDone, resp.Status)
	require.Equal(t, "https://example.com/final.mp4", resp.VideoURL)
	require.Equal(t, len(sequence), calls)
	require.Equal(t, []api.VideoPreset{api.VideoPresetT2V720, api.VideoPresetT2V720, api.VideoPresetT2V720}, seenPresets)
}

func TestVideoWait_Timeout(t *testing.T) {
	t.Parallel()

	calls := 0
	c := &Client{
		videoGetResult: func(context.Context, VideoGetResultRequest) (*VideoGetResultResponse, error) {
			calls++
			return &VideoGetResultResponse{Status: VideoStatusGenerating}, nil
		},
	}

	resp, err := c.VideoWait(context.Background(), "task-timeout", api.VideoPresetT2V1080, WaitOptions{
		Interval: time.Millisecond,
		Timeout:  15 * time.Millisecond,
	})
	require.Nil(t, resp)
	require.Error(t, err)
	require.Equal(t, internalerrors.ErrTimeout, internalerrors.GetCode(err))
	require.ErrorContains(t, err, "wait timeout")
	require.ErrorContains(t, err, "status=generating")
	require.GreaterOrEqual(t, calls, 2)
}

func TestVideoWait_TerminalFailureStates(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		status VideoStatus
	}{
		{name: "failed", status: VideoStatusFailed},
		{name: "not_found", status: VideoStatusNotFound},
		{name: "expired", status: VideoStatusExpired},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			c := &Client{
				videoGetResult: func(context.Context, VideoGetResultRequest) (*VideoGetResultResponse, error) {
					calls++
					return &VideoGetResultResponse{Status: tc.status}, nil
				},
			}

			resp, err := c.VideoWait(context.Background(), "task-terminal", api.VideoPresetI2VFirst, WaitOptions{
				Interval: time.Millisecond,
				Timeout:  time.Second,
			})
			require.Nil(t, resp)
			require.Error(t, err)
			require.Equal(t, internalerrors.ErrBusinessFailed, internalerrors.GetCode(err))
			require.ErrorContains(t, err, "video task ended with status")
			require.ErrorContains(t, err, string(tc.status))
			require.Equal(t, 1, calls)
		})
	}
}

func TestVideoErrorSemantics(t *testing.T) {
	t.Parallel()

	c := &Client{}

	t.Run("invalid preset returns validation with remediation", func(t *testing.T) {
		t.Parallel()

		_, err := c.SubmitVideoTask(context.Background(), VideoSubmitRequest{
			Preset:      api.VideoPreset("unknown-preset"),
			Prompt:      "animate subject",
			Frames:      121,
			AspectRatio: "16:9",
		})
		require.Error(t, err)
		require.Equal(t, internalerrors.ErrValidationFailed, internalerrors.GetCode(err))
		require.ErrorContains(t, err, "invalid preset")
		require.ErrorContains(t, err, "supported presets")
	})

	t.Run("req_key mismatch returns validation with expected req_key", func(t *testing.T) {
		t.Parallel()

		_, err := c.GetVideoResult(context.Background(), VideoGetResultRequest{
			TaskID: "task-video-1",
			Preset: api.VideoPresetI2VFirst,
			ReqKey: api.ReqKeyJimengT2VV30_1080p,
		})
		require.Error(t, err)
		require.Equal(t, internalerrors.ErrValidationFailed, internalerrors.GetCode(err))
		require.ErrorContains(t, err, "req_key mismatch")
		require.ErrorContains(t, err, api.ReqKeyJimengI2VFirstV30_1080)
	})

	t.Run("invalid i2v combination returns validation with hint", func(t *testing.T) {
		t.Parallel()

		_, err := c.SubmitVideoTask(context.Background(), VideoSubmitRequest{
			Preset:    api.VideoPresetI2VFirst,
			Prompt:    "animate this frame",
			ImageURLs: []string{"https://example.com/first.png"},
			Frames:    121,
		})
		require.Error(t, err)
		require.Equal(t, internalerrors.ErrValidationFailed, internalerrors.GetCode(err))
		require.ErrorContains(t, err, "i2v-first")
		require.ErrorContains(t, err, "without --frames/--aspect-ratio")
	})
}

func TestVideoI2VLocalPayloadTooLarge(t *testing.T) {
	t.Parallel()

	encoded := base64.StdEncoding.EncodeToString(make([]byte, maxVideoInlineImageBytes+1))

	c := &Client{}
	_, err := c.SubmitVideoTask(context.Background(), VideoSubmitRequest{
		Preset:    api.VideoPresetI2VFirst,
		Prompt:    "animate this local image",
		ImageURLs: []string{"data:image/png;base64," + strings.TrimSpace(encoded)},
	})
	require.Error(t, err)
	require.Equal(t, internalerrors.ErrValidationFailed, internalerrors.GetCode(err))
	require.ErrorContains(t, err, "local i2v image payload is too large")
	require.ErrorContains(t, err, "upload the image to a URL")
}
func TestVideoErrorDiagnostics_50400(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Simulate 50400 business failure
		_, _ = w.Write([]byte(`{"code":50400,"status":50400,"message":"access denied","request_id":"rid-50400"}`))
	}))
	defer server.Close()

	parsedURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	c, err := NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        parsedURL.Host,
		Scheme:      parsedURL.Scheme,
	})
	require.NoError(t, err)

	t.Run("submit path enrichment", func(t *testing.T) {
		_, err := c.SubmitVideoTask(context.Background(), VideoSubmitRequest{
			Preset:      api.VideoPresetT2V720,
			Prompt:      "test prompt",
			Frames:      121,
			AspectRatio: "16:9",
		})
		require.Error(t, err)
		// These assertions are expected to FAIL in the RED phase
		require.ErrorContains(t, err, "preset=t2v-720")
		require.ErrorContains(t, err, "req_key=jimeng_t2v_v30_720p")
		require.ErrorContains(t, err, "request_id=rid-50400")
		require.ErrorContains(t, err, "host="+parsedURL.Host)
		require.ErrorContains(t, err, "region=cn-north-1")
		require.ErrorContains(t, err, "service=cv")
		require.ErrorContains(t, err, "action=CVSync2AsyncSubmitTask")
		require.ErrorContains(t, err, "version=2022-08-31")
		require.ErrorContains(t, err, "classification=entitlement_or_scope_mismatch")
		require.ErrorContains(t, err, "next_steps=check_entitlement,verify_sigv4_scope,verify_req_key_for_preset,provide_request_id_to_support")
		require.ErrorContains(t, err, "runbook=server/README.md#50400-triage")
	})

	t.Run("query path enrichment", func(t *testing.T) {
		_, err := c.GetVideoResult(context.Background(), VideoGetResultRequest{
			TaskID: "task-123",
			Preset: api.VideoPresetT2V720,
			ReqKey: api.ReqKeyJimengT2VV30_720p,
		})
		require.Error(t, err)
		// These assertions are expected to FAIL in the RED phase
		require.ErrorContains(t, err, "preset=t2v-720")
		require.ErrorContains(t, err, "req_key=jimeng_t2v_v30_720p")
		require.ErrorContains(t, err, "request_id=rid-50400")
		require.ErrorContains(t, err, "host="+parsedURL.Host)
		require.ErrorContains(t, err, "region=cn-north-1")
		require.ErrorContains(t, err, "service=cv")
		require.ErrorContains(t, err, "action=CVSync2AsyncGetResult")
		require.ErrorContains(t, err, "version=2022-08-31")
		require.ErrorContains(t, err, "classification=entitlement_or_scope_mismatch")
		require.ErrorContains(t, err, "next_steps=check_entitlement,verify_sigv4_scope,verify_req_key_for_preset,provide_request_id_to_support")
		require.ErrorContains(t, err, "runbook=server/README.md#50400-triage")
	})

	t.Run("non-50400 errors are not enriched", func(t *testing.T) {
		server400 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":40000,"status":40000,"message":"bad request","request_id":"rid-40000"}`))
		}))
		defer server400.Close()
		parsedURL400, _ := url.Parse(server400.URL)
		c400, _ := NewClient(config.Config{
			Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
			Region:      "cn-north-1",
			Host:        parsedURL400.Host,
			Scheme:      parsedURL400.Scheme,
		})

		_, err := c400.SubmitVideoTask(context.Background(), VideoSubmitRequest{
			Preset:      api.VideoPresetT2V720,
			Prompt:      "test prompt",
			Frames:      121,
			AspectRatio: "16:9",
		})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "preset=")
		require.NotContains(t, err.Error(), "req_key=")
		require.NotContains(t, err.Error(), "host=")
		require.NotContains(t, err.Error(), "classification=entitlement_or_scope_mismatch")
		require.NotContains(t, err.Error(), "next_steps=")
		require.NotContains(t, err.Error(), "runbook=server/README.md#50400-triage")
	})

	t.Run("rate-limit errors are not misclassified as 50400 entitlement/scope", func(t *testing.T) {
		server50430 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":50430,"status":50430,"message":"rate limited","request_id":"rid-50430"}`))
		}))
		defer server50430.Close()

		parsedURL50430, err := url.Parse(server50430.URL)
		require.NoError(t, err)

		c50430, err := NewClient(config.Config{
			Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
			Region:      "cn-north-1",
			Host:        parsedURL50430.Host,
			Scheme:      parsedURL50430.Scheme,
		})
		require.NoError(t, err)

		_, err = c50430.GetVideoResult(context.Background(), VideoGetResultRequest{
			TaskID: "task-50430",
			Preset: api.VideoPresetT2V720,
			ReqKey: api.ReqKeyJimengT2VV30_720p,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "rate limited")
		require.NotContains(t, err.Error(), "classification=entitlement_or_scope_mismatch")
		require.NotContains(t, err.Error(), "next_steps=")
		require.NotContains(t, err.Error(), "runbook=server/README.md#50400-triage")
	})

	t.Run("validation errors are not misclassified as 50400 entitlement/scope", func(t *testing.T) {
		_, err := c.SubmitVideoTask(context.Background(), VideoSubmitRequest{
			Preset:      api.VideoPreset("invalid-preset"),
			Prompt:      "test prompt",
			Frames:      121,
			AspectRatio: "16:9",
		})
		require.Error(t, err)
		require.Equal(t, internalerrors.ErrValidationFailed, internalerrors.GetCode(err))
		require.NotContains(t, err.Error(), "classification=entitlement_or_scope_mismatch")
		require.NotContains(t, err.Error(), "next_steps=")
		require.NotContains(t, err.Error(), "runbook=server/README.md#50400-triage")
	})
}

func TestVideoValidation_I2VFirstTailAggregateSizeTooLarge(t *testing.T) {
	t.Parallel()

	// Each image is 4MiB, total 8MiB. 8MiB is less than 10MiB relay limit,
	// but we might want a stricter aggregate limit in the client to be safe.
	// For now, let's assume we want to enforce an aggregate limit of 8MiB or similar.
	// Actually, the task says "Cover first-tail aggregate-size edge case relevant to relay limits".
	// If relay limit is 10MiB, and we have two 5MiB images, that's 10MiB + overhead.
	// Let's test with two images that total > 10MiB but individually are <= 5MiB.
	size := 5 * 1024 * 1024 // 5MiB
	img1 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, size))
	img2 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, size))

	req := &VideoSubmitRequest{
		Variant:   VideoVariantI2VFirstTail,
		Prompt:    "animate two images",
		ImageURLs: []string{img1, img2},
	}

	err := ValidateVideoSubmitRequest(req)
	// This should fail because the aggregate size is too large for the relay.
	require.Error(t, err)
	require.ErrorContains(t, err, "aggregate local payload size is too large")
}

func TestVideoValidation_MalformedDataURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		url  string
		msg  string
	}{
		{
			name: "missing comma",
			url:  "data:image/png;base64;SGVsbG8=",
			msg:  "missing data URL separator",
		},
		{
			name: "empty base64",
			url:  "data:image/png;base64,",
			msg:  "base64 content is empty",
		},
		{
			name: "invalid base64 characters",
			url:  "data:image/png;base64,!!!",
			msg:  "invalid base64 content",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := &VideoSubmitRequest{
				Variant:   VideoVariantI2VFirstFrame,
				Prompt:    "test malformed data url",
				ImageURLs: []string{tc.url},
			}
			err := ValidateVideoSubmitRequest(req)
			require.Error(t, err)
			require.ErrorContains(t, err, tc.msg)
		})
	}
}

func TestVideoValidation_ValidDataURL(t *testing.T) {
	t.Parallel()

	req := &VideoSubmitRequest{
		Variant:   VideoVariantI2VFirstFrame,
		Prompt:    "test valid data url",
		ImageURLs: []string{"data:image/png;base64,SGVsbG8="}, // "Hello" in base64
	}
	err := ValidateVideoSubmitRequest(req)
	require.NoError(t, err)
}
