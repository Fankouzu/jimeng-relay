package jimeng

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jimeng-relay/client/internal/api"
	internalerrors "github.com/jimeng-relay/client/internal/errors"
)

type SubmitRequest struct {
	Prompt           string   `json:"prompt"`
	ImageURLs        []string `json:"image_urls"`
	BinaryDataBase64 []string `json:"binary_data_base64"`
	Size             string   `json:"size"`
	Width            int      `json:"width"`
	Height           int      `json:"height"`
	Scale            int      `json:"scale"`
	ForceSingle      bool     `json:"force_single"`
	MinRatio         string   `json:"min_ratio"`
	MaxRatio         string   `json:"max_ratio"`
}

type SubmitResponse struct {
	TaskID      string `json:"task_id"`
	Code        int    `json:"code"`
	Status      int    `json:"status"`
	Message     string `json:"message"`
	RequestID   string `json:"request_id"`
	TimeElapsed int    `json:"time_elapsed"`
}

func (c *Client) SubmitTask(ctx context.Context, req SubmitRequest) (*SubmitResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, internalerrors.New(internalerrors.ErrTimeout, "context done before submit", err)
	}

	if strings.TrimSpace(req.Prompt) == "" {
		return nil, internalerrors.New(internalerrors.ErrValidationFailed, "prompt is required", nil)
	}

	reqBody := map[string]interface{}{
		"req_key":    api.ReqKeyJimengT2IV40,
		"prompt":     req.Prompt,
		"return_url": true,
	}

	if imageURLs := CleanStringSlice(req.ImageURLs); len(imageURLs) > 0 {
		reqBody["image_urls"] = imageURLs
	}

	if binaryData := CleanStringSlice(req.BinaryDataBase64); len(binaryData) > 0 {
		reqBody["binary_data_base64"] = binaryData
	}

	if sizeRaw := strings.TrimSpace(req.Size); sizeRaw != "" {
		size, err := ParseInt(sizeRaw)
		if err != nil {
			return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("invalid size: %q", sizeRaw), err)
		}
		if size > 0 {
			reqBody["size"] = size
		}
	}

	if req.Width > 0 && req.Height > 0 {
		reqBody["width"] = req.Width
		reqBody["height"] = req.Height
	}

	if req.Scale != 0 {
		reqBody["scale"] = float64(req.Scale)
	}

	if req.ForceSingle {
		reqBody["force_single"] = true
	}

	if minRatioRaw := strings.TrimSpace(req.MinRatio); minRatioRaw != "" {
		minRatio, err := ParseFloat(minRatioRaw)
		if err != nil {
			return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("invalid min_ratio: %q", minRatioRaw), err)
		}
		reqBody["min_ratio"] = minRatio
	}
	if maxRatioRaw := strings.TrimSpace(req.MaxRatio); maxRatioRaw != "" {
		maxRatio, err := ParseFloat(maxRatioRaw)
		if err != nil {
			return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("invalid max_ratio: %q", maxRatioRaw), err)
		}
		reqBody["max_ratio"] = maxRatio
	}

	c.debug("submit request", "body", reqBody)

	var respBody map[string]interface{}
	retryCfg := DefaultRetryConfig
	retryCfg.MaxRetries = 6
	retryCfg.InitialDelay = time.Second
	retryCfg.MaxDelay = 20 * time.Second

	err := DoWithRetry(ctx, retryCfg, func() error {
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

	c.debug("response received", "has_error_field", respBody["error"] != nil, "code", respBody["code"], "status", respBody["status"], "full_body", respBody)

	// Check for relay server error format: {"error":{"code":"...","message":"..."}}
	if errObj, ok := respBody["error"].(map[string]interface{}); ok {
		errCode := ToString(errObj["code"])
		errMsg := ToString(errObj["message"])
		c.debug("relay error detected", "code", errCode, "message", errMsg)
		return nil, internalerrors.New(
			internalerrors.ErrAuthFailed,
			fmt.Sprintf("relay error: code=%s message=%s", errCode, errMsg),
			nil,
		)
	}

	code := ToInt(respBody["code"])
	status := ToInt(respBody["status"])
	message := ToString(respBody["message"])
	requestID := ToString(respBody["request_id"])
	timeElapsed := ToInt(respBody["time_elapsed"])

	if code != 10000 || status != 10000 {
		c.debug("business failure", "code", code, "status", status, "message", message, "request_id", requestID)
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
	if taskID == "" {
		return nil, internalerrors.New(
			internalerrors.ErrDecodeFailed,
			fmt.Sprintf("submit task returned empty task_id: code=%d status=%d message=%s request_id=%s", code, status, message, requestID),
			nil,
		)
	}

	resp := &SubmitResponse{
		TaskID:      taskID,
		Code:        code,
		Status:      status,
		Message:     message,
		RequestID:   requestID,
		TimeElapsed: timeElapsed,
	}
	c.debug("submit success", "task_id", taskID, "request_id", requestID)
	return resp, nil
}

