package jimeng

import (
	"context"
	"fmt"
	"strings"

	"github.com/jimeng-relay/client/internal/api"
	internalerrors "github.com/jimeng-relay/client/internal/errors"
)

type GetResultRequest struct {
	TaskID string
}

type TaskStatus string

const (
	StatusInQueue    TaskStatus = TaskStatus(api.StatusInQueue)
	StatusGenerating TaskStatus = TaskStatus(api.StatusGenerating)
	StatusDone       TaskStatus = TaskStatus(api.StatusDone)
	StatusNotFound   TaskStatus = TaskStatus(api.StatusNotFound)
	StatusExpired    TaskStatus = TaskStatus(api.StatusExpired)
	StatusFailed     TaskStatus = TaskStatus(api.StatusFailed)
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

	var body map[string]interface{}
	err := DoWithRetry(ctx, DefaultRetryConfig, func() error {
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

	resp := &GetResultResponse{
		Status:    TaskStatus(ToString(body["status"])),
		Code:      ToInt(body["code"]),
		Message:   ToString(body["message"]),
		RequestID: ToString(body["request_id"]),
	}

	if data, ok := body["data"].(map[string]interface{}); ok {
		if status := ToString(data["status"]); status != "" {
			resp.Status = TaskStatus(status)
		}
		resp.ImageURLs = ToStringSlice(data["image_urls"])
		resp.BinaryDataBase64 = ToStringSlice(data["binary_data_base64"])
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


func relayErrorToClientCode(code string) internalerrors.Code {
	switch strings.TrimSpace(code) {
	case "AUTH_FAILED", "KEY_REVOKED", "KEY_EXPIRED", "INVALID_SIGNATURE":
		return internalerrors.ErrAuthFailed
	case "RATE_LIMITED":
		return internalerrors.ErrRateLimited
	case "VALIDATION_FAILED":
		return internalerrors.ErrValidationFailed
	default:
		return internalerrors.ErrUnknown
	}
}


