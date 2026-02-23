package jimeng

import (
	"fmt"

	internalerrors "github.com/jimeng-relay/client/internal/errors"
)

const (
	MaxPromptLength = 2000
	MinWidth        = 256
	MaxWidth        = 4096
	MinHeight       = 256
	MaxHeight       = 4096
	MaxImageURLs    = 10
	MinScale        = 0.0
	MaxScale        = 2.0
)

func ValidateSubmitRequest(req *SubmitRequest) error {
	if req == nil {
		return internalerrors.New(internalerrors.ErrValidationFailed, "request is nil", nil)
	}

	if req.Prompt == "" {
		return internalerrors.New(internalerrors.ErrValidationFailed, "prompt is required", nil)
	}
	if len(req.Prompt) > MaxPromptLength {
		return internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("prompt length exceeds maximum of %d characters", MaxPromptLength), nil)
	}

	if req.Width != 0 {
		if req.Width < MinWidth || req.Width > MaxWidth {
			return internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("width must be in range [%d, %d]", MinWidth, MaxWidth), nil)
		}
	}

	if req.Height != 0 {
		if req.Height < MinHeight || req.Height > MaxHeight {
			return internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("height must be in range [%d, %d]", MinHeight, MaxHeight), nil)
		}
	}

	if req.Scale != 0 {
		if float64(req.Scale) < MinScale || float64(req.Scale) > MaxScale {
			return internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("scale must be in range [%.1f, %.1f]", MinScale, MaxScale), nil)
		}
	}

	if len(req.ImageURLs) > MaxImageURLs {
		return internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("image_urls count exceeds maximum of %d", MaxImageURLs), nil)
	}

	if len(req.BinaryDataBase64) > MaxImageURLs {
		return internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("binary_data_base64 count exceeds maximum of %d", MaxImageURLs), nil)
	}

	if len(req.ImageURLs) > 0 && len(req.BinaryDataBase64) > 0 {
		return internalerrors.New(internalerrors.ErrValidationFailed, "image_urls and binary_data_base64 cannot be used together", nil)
	}

	return nil
}
