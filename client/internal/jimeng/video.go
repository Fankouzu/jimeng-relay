package jimeng

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jimeng-relay/client/internal/api"
	internalerrors "github.com/jimeng-relay/client/internal/errors"
)

type VideoVariant string

const (
	VideoVariantT2V           VideoVariant = "t2v"
	VideoVariantI2VFirstFrame VideoVariant = "i2v-first-frame"
	VideoVariantI2VFirstTail  VideoVariant = "i2v-first-tail"
	VideoVariantRecamera      VideoVariant = "i2v-recamera"
)

type VideoCameraStrength string

const (
	VideoCameraStrengthWeak   VideoCameraStrength = "weak"
	VideoCameraStrengthMedium VideoCameraStrength = "medium"
	VideoCameraStrengthStrong VideoCameraStrength = "strong"
)

type VideoSubmitRequest struct {
	Variant        VideoVariant    `json:"variant"`
	Preset         api.VideoPreset `json:"preset"`
	ReqKey         string          `json:"req_key"`
	Prompt         string          `json:"prompt"`
	Frames         int             `json:"frames"`
	AspectRatio    string          `json:"aspect_ratio"`
	Seed           int             `json:"seed"`
	ImageURLs      []string        `json:"image_urls"`
	TemplateID     string          `json:"template_id"`
	CameraStrength string          `json:"camera_strength"`
}

type VideoSubmitResponse struct {
	TaskID      string          `json:"task_id"`
	Preset      api.VideoPreset `json:"preset"`
	ReqKey      string          `json:"req_key"`
	Code        int             `json:"code"`
	Status      int             `json:"status"`
	Message     string          `json:"message"`
	RequestID   string          `json:"request_id"`
	TimeElapsed int             `json:"time_elapsed"`
}

type VideoGetResultRequest struct {
	TaskID string          `json:"task_id"`
	Preset api.VideoPreset `json:"preset"`
	ReqKey string          `json:"req_key"`
}

type VideoStatus string

const (
	VideoStatusInQueue    VideoStatus = VideoStatus(api.StatusInQueue)
	VideoStatusGenerating VideoStatus = VideoStatus(api.StatusGenerating)
	VideoStatusDone       VideoStatus = VideoStatus(api.StatusDone)
	VideoStatusNotFound   VideoStatus = VideoStatus(api.StatusNotFound)
	VideoStatusExpired    VideoStatus = VideoStatus(api.StatusExpired)
	VideoStatusFailed     VideoStatus = VideoStatus(api.StatusFailed)
)

type VideoGetResultResponse struct {
	Preset    api.VideoPreset `json:"preset"`
	ReqKey    string          `json:"req_key"`
	Status    VideoStatus     `json:"status"`
	VideoURL  string          `json:"video_url"`
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	RequestID string          `json:"request_id"`
}

func (c *Client) SubmitVideoTask(ctx context.Context, req VideoSubmitRequest) (*VideoSubmitResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, internalerrors.New(internalerrors.ErrTimeout, "context done before submit", err)
	}

	reqKey, err := api.VideoSubmitReqKey(req.Preset)
	if err != nil {
		return nil, internalerrors.New(internalerrors.ErrValidationFailed, invalidVideoPresetMessage(req.Preset), err)
	}

	if requestReqKey := strings.TrimSpace(req.ReqKey); requestReqKey != "" && requestReqKey != reqKey {
		return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("req_key mismatch: expected %s, got %s", reqKey, req.ReqKey), nil)
	}

	expectedVariant, err := videoVariantForPreset(req.Preset)
	if err != nil {
		return nil, err
	}
	if req.Variant == "" {
		req.Variant = expectedVariant
	} else if req.Variant != expectedVariant {
		return nil, internalerrors.New(
			internalerrors.ErrValidationFailed,
			fmt.Sprintf("variant mismatch for preset %q: expected %q, got %q", req.Preset, expectedVariant, req.Variant),
			nil,
		)
	}

	if err := ValidateVideoSubmitRequest(&req); err != nil {
		return nil, err
	}

	reqBody := map[string]interface{}{
		"req_key":    reqKey,
		"prompt":     req.Prompt,
		"return_url": true,
	}

	if req.Frames > 0 {
		reqBody["frames"] = req.Frames
	}
	if req.AspectRatio != "" {
		reqBody["aspect_ratio"] = req.AspectRatio
	}
	if req.Seed != 0 {
		reqBody["seed"] = req.Seed
	}
	if len(req.ImageURLs) > 0 {
		reqBody["image_urls"] = req.ImageURLs
	}
	if req.TemplateID != "" {
		reqBody["template_id"] = req.TemplateID
	}
	if req.CameraStrength != "" {
		reqBody["camera_strength"] = req.CameraStrength
	}

	c.debug("video submit request", "body", reqBody)

	var respBody map[string]interface{}
	retryCfg := DefaultRetryConfig
	retryCfg.MaxRetries = 6
	retryCfg.InitialDelay = time.Second
	retryCfg.MaxDelay = 20 * time.Second

	err = DoWithRetry(ctx, retryCfg, func() error {
		body, _, callErr := c.visual.CVSync2AsyncSubmitTask(reqBody)
		c.debug("API call completed", "error", callErr)
		if callErr != nil {
			return internalerrors.New(internalerrors.ErrUnknown, "submit task request failed", callErr)
		}

		code := submitToInt(body["code"])
		status := submitToInt(body["status"])
		if code == 50429 || code == 50430 || status == 50429 || status == 50430 {
			message := submitToString(body["message"])
			requestID := submitToString(body["request_id"])
			return internalerrors.New(
				internalerrors.ErrRateLimited,
				fmt.Sprintf("submit task rate limited: code=%d status=%d message=%s request_id=%s", code, status, message, requestID),
				nil,
			)
		}

		respBody = body
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, internalerrors.New(internalerrors.ErrTimeout, "context done after submit", err)
	}

	code := submitToInt(respBody["code"])
	status := submitToInt(respBody["status"])
	message := submitToString(respBody["message"])
	requestID := submitToString(respBody["request_id"])
	timeElapsed := submitToInt(respBody["time_elapsed"])

	if code != 10000 || status != 10000 {
		return nil, internalerrors.New(
			internalerrors.ErrBusinessFailed,
			fmt.Sprintf("submit task business failed: code=%d status=%d message=%s request_id=%s", code, status, message, requestID),
			nil,
		)
	}

	taskID := ""
	if data, ok := respBody["data"].(map[string]interface{}); ok {
		taskID = strings.TrimSpace(submitToString(data["task_id"]))
	}
	if taskID == "" {
		taskID = strings.TrimSpace(submitToString(respBody["task_id"]))
	}

	return &VideoSubmitResponse{
		TaskID:      taskID,
		Preset:      req.Preset,
		ReqKey:      reqKey,
		Code:        code,
		Status:      status,
		Message:     message,
		RequestID:   requestID,
		TimeElapsed: timeElapsed,
	}, nil
}

func (c *Client) GetVideoResult(ctx context.Context, req VideoGetResultRequest) (*VideoGetResultResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if req.TaskID == "" {
		return nil, fmt.Errorf("%s: task_id is required", api.ActionGetResult)
	}

	reqKey, err := api.VideoQueryReqKey(req.Preset)
	if err != nil {
		return nil, internalerrors.New(internalerrors.ErrValidationFailed, invalidVideoPresetMessage(req.Preset), err)
	}

	requestReqKey := strings.TrimSpace(req.ReqKey)
	if requestReqKey == "" {
		return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("req_key is required for preset %q", req.Preset), nil)
	}
	if requestReqKey != reqKey {
		return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("req_key mismatch: expected %s, got %s", reqKey, req.ReqKey), nil)
	}

	reqBody := map[string]interface{}{
		"req_key": reqKey,
		"task_id": req.TaskID,
	}

	var body map[string]interface{}
	err = DoWithRetry(ctx, DefaultRetryConfig, func() error {
		respBody, _, reqErr := c.visual.CVSync2AsyncGetResult(reqBody)
		if reqErr != nil {
			return reqErr
		}

		if errObj, ok := respBody["error"].(map[string]interface{}); ok {
			errCode := strings.TrimSpace(toString(errObj["code"]))
			errMsg := strings.TrimSpace(toString(errObj["message"]))
			return internalerrors.New(
				relayErrorToClientCode(errCode),
				fmt.Sprintf("relay error: code=%s message=%s", errCode, errMsg),
				nil,
			)
		}

		code := toInt(respBody["code"])
		status := toInt(respBody["status"])
		message := toString(respBody["message"])
		requestID := toString(respBody["request_id"])

		if code == 0 && status == 0 && message == "" && requestID == "" {
			return internalerrors.New(
				internalerrors.ErrDecodeFailed,
				"query task response missing code/status/message/request_id",
				nil,
			)
		}

		if code == 50429 || code == 50430 || status == 50429 || status == 50430 {
			return internalerrors.New(
				internalerrors.ErrRateLimited,
				fmt.Sprintf("query task rate limited: code=%d status=%d message=%s request_id=%s", code, status, message, requestID),
				nil,
			)
		}

		if code != 10000 || status != 10000 {
			return internalerrors.New(
				internalerrors.ErrBusinessFailed,
				fmt.Sprintf("query task business failed: code=%d status=%d message=%s request_id=%s", code, status, message, requestID),
				nil,
			)
		}

		body = respBody
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", api.ActionGetResult, err)
	}

	resp := &VideoGetResultResponse{
		Preset:    req.Preset,
		ReqKey:    reqKey,
		Status:    VideoStatus(toString(body["status"])),
		Code:      toInt(body["code"]),
		Message:   toString(body["message"]),
		RequestID: toString(body["request_id"]),
	}

	if data, ok := body["data"].(map[string]interface{}); ok {
		if status := toString(data["status"]); status != "" {
			resp.Status = VideoStatus(status)
		}
		resp.VideoURL = toString(data["video_url"])
	}

	return resp, nil
}

func videoVariantForPreset(preset api.VideoPreset) (VideoVariant, error) {
	switch preset {
	case api.VideoPresetT2V720, api.VideoPresetT2V1080:
		return VideoVariantT2V, nil
	case api.VideoPresetI2VFirst:
		return VideoVariantI2VFirstFrame, nil
	case api.VideoPresetI2VFirstTail:
		return VideoVariantI2VFirstTail, nil
	case api.VideoPresetI2VRecamera:
		return VideoVariantRecamera, nil
	default:
		return "", internalerrors.New(internalerrors.ErrValidationFailed, invalidVideoPresetMessage(preset), nil)
	}
}

func invalidVideoPresetMessage(preset api.VideoPreset) string {
	return fmt.Sprintf("invalid preset %q; supported presets: %q, %q, %q, %q, %q", preset, api.VideoPresetT2V720, api.VideoPresetT2V1080, api.VideoPresetI2VFirst, api.VideoPresetI2VFirstTail, api.VideoPresetI2VRecamera)
}
