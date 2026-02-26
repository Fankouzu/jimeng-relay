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
			_, _ = w.Write([]byte(`{"code":10000,"status":10000,"message":"success","request_id":"rid-submit","time_elapsed":12,"data":{"task_id":"task-video-1"}}`))
		case api.ActionGetResult:
			_, _ = w.Write([]byte(`{"code":10000,"status":10000,"message":"success","request_id":"rid-query","data":{"status":"done","video_url":"https://example.com/video.mp4"}}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":400,"status":400,"message":"unknown action"}`))
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
