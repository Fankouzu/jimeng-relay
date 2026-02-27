package jimeng

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
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
	Variant           VideoVariant    `json:"variant"`
	Preset            api.VideoPreset `json:"preset"`
	ReqKey            string          `json:"req_key"`
	Prompt            string          `json:"prompt"`
	Frames            int             `json:"frames"`
	AspectRatio       string          `json:"aspect_ratio"`
	Seed              int             `json:"seed"`
	ImageURLs         []string        `json:"image_urls"`
	BinaryDataBase64  []string        `json:"binary_data_base64"`
	TemplateID        string          `json:"template_id"`
	CameraStrength    string          `json:"camera_strength"`
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

	if len(req.ImageURLs) > 0 || len(req.BinaryDataBase64) > 0 {
		// Combine ImageURLs and BinaryDataBase64 for processing
		allInputs := make([]string, 0, len(req.ImageURLs)+len(req.BinaryDataBase64))
		allInputs = append(allInputs, req.ImageURLs...)
		allInputs = append(allInputs, req.BinaryDataBase64...)

		prepared, err := prepareVideoImages(allInputs)
		if err != nil {
			return nil, err
		}
		if prepared != nil {
			req.ImageURLs = prepared.imageURLs
			req.BinaryDataBase64 = prepared.binaryDataBase64
		}
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
	if len(req.BinaryDataBase64) > 0 {
		reqBody["binary_data_base64"] = req.BinaryDataBase64
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

		code := ToInt(body["code"])
		status := ToInt(body["status"])
		if code == 50429 || code == 50430 || status == 50429 || status == 50430 {
			message := ToString(body["message"])
			requestID := ToString(body["request_id"])
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

	code := ToInt(respBody["code"])
	status := ToInt(respBody["status"])
	message := ToString(respBody["message"])
	requestID := ToString(respBody["request_id"])
	timeElapsed := ToInt(respBody["time_elapsed"])

	if code != 10000 || status != 10000 {
		if code == 50400 || status == 50400 {
			return nil, internalerrors.New(
				internalerrors.ErrBusinessFailed,
				video50400ErrorMessage(videoErrorDiagnosticContext{
					operation: "submit task business failed",
					preset:    req.Preset,
					reqKey:    reqKey,
					host:      c.config.Host,
					region:    c.config.Region,
					action:    api.ActionSubmitTask,
					code:      code,
					status:    status,
					message:   message,
					requestID: requestID,
				}),
				nil,
			)
		}
		return nil, internalerrors.New(
			internalerrors.ErrBusinessFailed,
			fmt.Sprintf("submit task business failed: code=%d status=%d message=%s request_id=%s", code, status, message, requestID),
			nil,
		)
	}

	taskID := ""
	if data, ok := respBody["data"].(map[string]interface{}); ok {
		taskID = strings.TrimSpace(ToString(data["task_id"]))
	}
	if taskID == "" {
		taskID = strings.TrimSpace(ToString(respBody["task_id"]))
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

// preparedVideoImages holds the processed image data for video generation
type preparedVideoImages struct {
	imageURLs        []string
	binaryDataBase64 []string
}

func prepareVideoImages(raw []string) (*preparedVideoImages, error) {
	urls := CleanStringSlice(raw)
	if len(urls) == 0 {
		return nil, nil
	}

	result := &preparedVideoImages{
		imageURLs:        make([]string, 0, len(urls)),
		binaryDataBase64: make([]string, 0, len(urls)),
	}

	for _, u := range urls {
		lower := strings.ToLower(u)

		// HTTP/HTTPS URLs go to image_urls
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			result.imageURLs = append(result.imageURLs, u)
			continue
		}

// Data URLs: extract base64 content and put in binary_data_base64
if strings.HasPrefix(lower, "data:image/") {
comma := strings.Index(u, ",")
if comma < 0 {
return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("invalid data URL format for image: missing base64 content"), nil)
}
base64Content := u[comma+1:]
if base64Content == "" {
return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("invalid data URL format for image: empty base64 content"), nil)
			}
			// Validate base64 content size
			if estimatedBase64DecodedLength(base64Content) > maxVideoInlineImageBytes {
				return nil, internalerrors.New(
					internalerrors.ErrValidationFailed,
					fmt.Sprintf("local i2v image payload is too large (max %d bytes after decode); upload the image to a URL or compress it before submit", maxVideoInlineImageBytes),
					nil,
				)
			}
result.binaryDataBase64 = append(result.binaryDataBase64, base64Content)
continue
}

		// Check if it's a local file path
		isPathLike := strings.HasPrefix(u, "./") || strings.HasPrefix(u, "../") || strings.HasPrefix(u, "/")
		if !isPathLike {
			if _, err := os.Stat(u); err != nil {
				// Not a file, treat as URL (will be validated by server)
				result.imageURLs = append(result.imageURLs, u)
				continue
			}
		}

		// It's a local file - read and encode to base64
		info, err := os.Stat(u)
		if err != nil {
			return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("stat --image-file %q failed", u), err)
		}
		if info.IsDir() {
			return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("--image-file %q is a directory", u), nil)
		}
		if info.Size() <= 0 {
			return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("--image-file %q is empty", u), nil)
		}
		if info.Size() > int64(maxVideoInlineImageBytes) {
			return nil, internalerrors.New(
				internalerrors.ErrValidationFailed,
				fmt.Sprintf("--image-file %q exceeds max size %d bytes; compress it or upload it to a URL", u, maxVideoInlineImageBytes),
				nil,
			)
		}

		data, err := os.ReadFile(u)
		if err != nil {
			return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("read --image-file %q failed", u), err)
		}

		encoded := base64.StdEncoding.EncodeToString(data)
		result.binaryDataBase64 = append(result.binaryDataBase64, encoded)
	}

	return result, nil
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
			errCode := strings.TrimSpace(ToString(errObj["code"]))
			errMsg := strings.TrimSpace(ToString(errObj["message"]))
			return internalerrors.New(
				relayErrorToClientCode(errCode),
				fmt.Sprintf("relay error: code=%s message=%s", errCode, errMsg),
				nil,
			)
		}

		code := ToInt(respBody["code"])
		status := ToInt(respBody["status"])
		message := ToString(respBody["message"])
		requestID := ToString(respBody["request_id"])

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
			if code == 50400 || status == 50400 {
				return internalerrors.New(
					internalerrors.ErrBusinessFailed,
					video50400ErrorMessage(videoErrorDiagnosticContext{
						operation: "query task business failed",
						preset:    req.Preset,
						reqKey:    reqKey,
						host:      c.config.Host,
						region:    c.config.Region,
						action:    api.ActionGetResult,
						code:      code,
						status:    status,
						message:   message,
						requestID: requestID,
					}),
					nil,
				)
			}
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
		Status:    VideoStatus(ToString(body["status"])),
		Code:      ToInt(body["code"]),
		Message:   ToString(body["message"]),
		RequestID: ToString(body["request_id"]),
	}

	if data, ok := body["data"].(map[string]interface{}); ok {
		if status := ToString(data["status"]); status != "" {
			resp.Status = VideoStatus(status)
		}
		resp.VideoURL = ToString(data["video_url"])
	}

	return resp, nil
}

func videoVariantForPreset(preset api.VideoPreset) (VideoVariant, error) {
	switch preset {
	case api.VideoPresetT2V720, api.VideoPresetT2V1080, api.VideoPresetT2VPro:
		return VideoVariantT2V, nil
	case api.VideoPresetI2VFirst720, api.VideoPresetI2VFirst, api.VideoPresetI2VFirstPro:
		return VideoVariantI2VFirstFrame, nil
	case api.VideoPresetI2VFirstTail720, api.VideoPresetI2VFirstTail:
		return VideoVariantI2VFirstTail, nil
	case api.VideoPresetI2VRecamera:
		return VideoVariantRecamera, nil
	default:
		return "", internalerrors.New(internalerrors.ErrValidationFailed, invalidVideoPresetMessage(preset), nil)
	}
}

func invalidVideoPresetMessage(preset api.VideoPreset) string {
	return fmt.Sprintf("invalid preset %q; supported presets: %q, %q, %q, %q, %q, %q, %q, %q, %q", preset, api.VideoPresetT2V720, api.VideoPresetT2V1080, api.VideoPresetT2VPro, api.VideoPresetI2VFirst720, api.VideoPresetI2VFirst, api.VideoPresetI2VFirstPro, api.VideoPresetI2VFirstTail720, api.VideoPresetI2VFirstTail, api.VideoPresetI2VRecamera)
}

type videoErrorDiagnosticContext struct {
	operation string
	preset    api.VideoPreset
	reqKey    string
	host      string
	region    string
	action    string
	code      int
	status    int
	message   string
	requestID string
}

func video50400ErrorMessage(ctx videoErrorDiagnosticContext) string {
	return fmt.Sprintf(
		"%s: code=%d status=%d message=%s preset=%s req_key=%s request_id=%s host=%s region=%s service=cv action=%s version=2022-08-31 classification=entitlement_or_scope_mismatch next_steps=check_entitlement,verify_sigv4_scope,verify_req_key_for_preset,provide_request_id_to_support runbook=server/README.md#50400-triage",
		ctx.operation,
		ctx.code,
		ctx.status,
		ctx.message,
		ctx.preset,
		ctx.reqKey,
		ctx.requestID,
		ctx.host,
		ctx.region,
		ctx.action,
	)
}
