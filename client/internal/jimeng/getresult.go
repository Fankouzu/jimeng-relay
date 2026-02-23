package jimeng

import (
	"context"
	"fmt"

	"github.com/jimeng-relay/client/internal/api"
)

type GetResultRequest struct {
	TaskID string
}

type TaskStatus string

const (
	StatusInQueue   TaskStatus = TaskStatus(api.StatusInQueue)
	StatusGenerating TaskStatus = TaskStatus(api.StatusGenerating)
	StatusDone      TaskStatus = TaskStatus(api.StatusDone)
	StatusNotFound  TaskStatus = TaskStatus(api.StatusNotFound)
	StatusExpired   TaskStatus = TaskStatus(api.StatusExpired)
	StatusFailed    TaskStatus = TaskStatus(api.StatusFailed)
)

type GetResultResponse struct {
	Status           TaskStatus
	ImageURLs        []string
	BinaryDataBase64 []string
	Code             int
	Message          string
	RequestID        string
}

func (c *Client) GetResult(ctx context.Context, req GetResultRequest) (*GetResultResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if req.TaskID == "" {
		return nil, fmt.Errorf("%s: task_id is required", api.ActionGetResult)
	}

	reqBody := map[string]interface{}{
		"req_key": api.ReqKeyJimengT2IV40,
		"task_id": req.TaskID,
	}

	body, _, err := c.visual.CVSync2AsyncGetResult(reqBody)
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", api.ActionGetResult, err)
	}

	resp := &GetResultResponse{
		Status:    TaskStatus(toString(body["status"])),
		Code:      toInt(body["code"]),
		Message:   toString(body["message"]),
		RequestID: toString(body["request_id"]),
	}

	if data, ok := body["data"].(map[string]interface{}); ok {
		if status := toString(data["status"]); status != "" {
			resp.Status = TaskStatus(status)
		}
		resp.ImageURLs = toStringSlice(data["image_urls"])
		resp.BinaryDataBase64 = toStringSlice(data["binary_data_base64"])
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return resp, nil
}

func (r *GetResultResponse) IsTerminal() bool {
	if r == nil {
		return false
	}

	switch r.Status {
	case StatusDone, StatusNotFound, StatusExpired, StatusFailed:
		return true
	default:
		return false
	}
}

func toString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}

	if ss, ok := v.([]string); ok {
		return ss
	}

	items, ok := v.([]interface{})
	if !ok {
		return nil
	}

	res := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			res = append(res, s)
		}
	}

	return res
}
