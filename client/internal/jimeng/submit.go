package jimeng

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jimeng-relay/client/internal/api"
	internalerrors "github.com/jimeng-relay/client/internal/errors"
)

type SubmitRequest struct {
	Prompt      string   `json:"prompt"`
	ImageURLs   []string `json:"image_urls"`
	BinaryDataBase64 []string `json:"binary_data_base64"`
	Size        string   `json:"size"`
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	Scale       int      `json:"scale"`
	ForceSingle bool     `json:"force_single"`
	MinRatio    string   `json:"min_ratio"`
	MaxRatio    string   `json:"max_ratio"`
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

	if imageURLs := submitCleanStringSlice(req.ImageURLs); len(imageURLs) > 0 {
		reqBody["image_urls"] = imageURLs
	}

	if binaryData := submitCleanStringSlice(req.BinaryDataBase64); len(binaryData) > 0 {
		reqBody["binary_data_base64"] = binaryData
	}

	if sizeRaw := strings.TrimSpace(req.Size); sizeRaw != "" {
		size, err := submitParseInt(sizeRaw)
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
		minRatio, err := submitParseFloat(minRatioRaw)
		if err != nil {
			return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("invalid min_ratio: %q", minRatioRaw), err)
		}
		reqBody["min_ratio"] = minRatio
	}
	if maxRatioRaw := strings.TrimSpace(req.MaxRatio); maxRatioRaw != "" {
		maxRatio, err := submitParseFloat(maxRatioRaw)
		if err != nil {
			return nil, internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("invalid max_ratio: %q", maxRatioRaw), err)
		}
		reqBody["max_ratio"] = maxRatio
	}

	respBody, _, err := c.visual.CVSync2AsyncSubmitTask(reqBody)
	if err != nil {
		return nil, internalerrors.New(internalerrors.ErrUnknown, "submit task request failed", err)
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
	return resp, nil
}

func submitToString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%.0f", val)
	case int:
		return fmt.Sprintf("%d", val)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func submitToInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

func submitCleanStringSlice(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	out := make([]string, 0, len(v))
	for _, s := range v {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func submitParseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func submitParseFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}

	if strings.Contains(s, "/") {
		parts := strings.SplitN(s, "/", 2)
		if len(parts) != 2 {
			return 0, fmt.Errorf("invalid fraction")
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return 0, err
		}
		d, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return 0, err
		}
		if d == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return n / d, nil
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}
