package jimeng

import (
	"fmt"
	"strings"

	"github.com/jimeng-relay/client/internal/api"
	internalerrors "github.com/jimeng-relay/client/internal/errors"
)

var validVideoAspectRatios = map[string]struct{}{
	"16:9": {},
	"4:3":  {},
	"1:1":  {},
	"3:4":  {},
	"9:16": {},
	"21:9": {},
}

var validVideoCameraStrength = map[string]struct{}{
	string(VideoCameraStrengthWeak):   {},
	string(VideoCameraStrengthMedium): {},
	string(VideoCameraStrengthStrong): {},
}

const (
	MaxPromptLength          = 2000
	MinWidth                 = 256
	MaxWidth                 = 4096
	MinHeight                = 256
	MaxHeight                = 4096
	MaxImageURLs             = 10
	maxVideoInlineImageBytes = 5 * 1024 * 1024
	MinScale                 = 0.0
	MaxScale                 = 2.0
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

func ValidateVideoSubmitRequest(req *VideoSubmitRequest) error {
	if req == nil {
		return internalerrors.New(internalerrors.ErrValidationFailed, "request is nil", nil)
	}

	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return internalerrors.New(internalerrors.ErrValidationFailed, "prompt is required", nil)
	}
	if len(req.Prompt) > MaxPromptLength {
		return internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("prompt length exceeds maximum of %d characters", MaxPromptLength), nil)
	}

	req.AspectRatio = strings.TrimSpace(req.AspectRatio)
	req.TemplateID = strings.TrimSpace(req.TemplateID)
	req.CameraStrength = strings.TrimSpace(req.CameraStrength)
	req.ImageURLs = CleanStringSlice(req.ImageURLs)
	req.BinaryDataBase64 = CleanStringSlice(req.BinaryDataBase64)

	totalImageCount := len(req.ImageURLs) + len(req.BinaryDataBase64)
	if req.Variant == "" && req.Preset != "" {
		switch req.Preset {
		case api.VideoPresetT2V720, api.VideoPresetT2V1080, api.VideoPresetT2VPro:
			req.Variant = VideoVariantT2V
		case api.VideoPresetI2VFirst720, api.VideoPresetI2VFirst, api.VideoPresetI2VFirstPro:
			req.Variant = VideoVariantI2VFirstFrame
		case api.VideoPresetI2VFirstTail720, api.VideoPresetI2VFirstTail:
			req.Variant = VideoVariantI2VFirstTail
		case api.VideoPresetI2VRecamera:
			req.Variant = VideoVariantRecamera
		}
	}

	switch req.Variant {

	case VideoVariantT2V:
		if req.Frames != 121 && req.Frames != 241 {
			return internalerrors.New(internalerrors.ErrValidationFailed, "frames must be 121 or 241", nil)
		}
		if _, ok := validVideoAspectRatios[req.AspectRatio]; !ok {
			return internalerrors.New(internalerrors.ErrValidationFailed, "aspect_ratio is invalid", nil)
		}
		if totalImageCount > 0 {
			return internalerrors.New(internalerrors.ErrValidationFailed, "image_urls and binary_data_base64 are not allowed for t2v", nil)
		}
		if req.TemplateID != "" {
			return internalerrors.New(internalerrors.ErrValidationFailed, "template_id is not allowed for t2v", nil)
		}
		if req.CameraStrength != "" {
			return internalerrors.New(internalerrors.ErrValidationFailed, "camera_strength is not allowed for t2v", nil)
		}
	case VideoVariantI2VFirstFrame:
		if err := validateVideoInlineImagePayloads(req.ImageURLs); err != nil {
			return err
		}
		if totalImageCount != 1 {
			return internalerrors.New(internalerrors.ErrValidationFailed, "i2v first-frame requires exactly 1 image", nil)
		}
		if req.TemplateID != "" {
			return internalerrors.New(internalerrors.ErrValidationFailed, "template_id is not allowed for i2v first-frame", nil)
		}
		if req.CameraStrength != "" {
			return internalerrors.New(internalerrors.ErrValidationFailed, "camera_strength is not allowed for i2v first-frame", nil)
		}
		if req.Frames != 0 || req.AspectRatio != "" {
			return internalerrors.New(internalerrors.ErrValidationFailed, "frames/aspect_ratio are not allowed for i2v first-frame; use i2v-first with exactly 1 image and without --frames/--aspect-ratio", nil)
		}
	case VideoVariantI2VFirstTail:
		if err := validateVideoInlineImagePayloads(req.ImageURLs); err != nil {
			return err
		}
		if totalImageCount != 2 {
			return internalerrors.New(internalerrors.ErrValidationFailed, "i2v first-tail requires exactly 2 images", nil)
		}
		if total := estimatedVideoInlineImageAggregateDecodedBytes(req.ImageURLs); total >= 2*maxVideoInlineImageBytes {
			return internalerrors.New(
				internalerrors.ErrValidationFailed,
				fmt.Sprintf("aggregate local payload size is too large (max %d bytes after decode); upload the images to URLs or compress them before submit", 2*maxVideoInlineImageBytes),
				nil,
			)
		}
		if req.TemplateID != "" {
			return internalerrors.New(internalerrors.ErrValidationFailed, "template_id is not allowed for i2v first-tail", nil)
		}
		if req.CameraStrength != "" {
			return internalerrors.New(internalerrors.ErrValidationFailed, "camera_strength is not allowed for i2v first-tail", nil)
		}
		if req.Frames != 0 || req.AspectRatio != "" {
			return internalerrors.New(internalerrors.ErrValidationFailed, "frames/aspect_ratio are not allowed for i2v first-tail", nil)
		}
	case VideoVariantRecamera:
		if err := validateVideoInlineImagePayloads(req.ImageURLs); err != nil {
			return err
		}
		if totalImageCount != 1 {
			return internalerrors.New(internalerrors.ErrValidationFailed, "i2v recamera requires exactly 1 image", nil)
		}
		if req.TemplateID == "" {
			return internalerrors.New(internalerrors.ErrValidationFailed, "template_id is required for recamera", nil)
		}
		if req.CameraStrength != "" {
			if _, ok := validVideoCameraStrength[req.CameraStrength]; !ok {
				return internalerrors.New(internalerrors.ErrValidationFailed, "camera_strength is invalid", nil)
			}
		}
		if req.Frames != 0 || req.AspectRatio != "" {
			return internalerrors.New(internalerrors.ErrValidationFailed, "frames/aspect_ratio are not allowed for recamera", nil)
		}
	default:
		return internalerrors.New(internalerrors.ErrValidationFailed, fmt.Sprintf("variant is invalid: %q", req.Variant), nil)
	}

	return nil
}

func estimatedVideoInlineImageAggregateDecodedBytes(imageURLs []string) int {
	total := 0
	for _, raw := range imageURLs {
		trimmed := strings.TrimSpace(raw)
		if !strings.HasPrefix(strings.ToLower(trimmed), "data:image/") {
			continue
		}
		comma := strings.Index(trimmed, ",")
		if comma < 0 {
			continue
		}
		payload := stripBase64Whitespace(trimmed[comma+1:])
		total += estimatedBase64DecodedLength(payload)
	}
	return total
}

func validateVideoInlineImagePayloads(imageURLs []string) error {
	for _, raw := range imageURLs {
		trimmed := strings.TrimSpace(raw)
		if !strings.HasPrefix(strings.ToLower(trimmed), "data:image/") {
			continue
		}

		comma := strings.Index(trimmed, ",")
		if comma < 0 {
			return internalerrors.New(internalerrors.ErrValidationFailed, "invalid i2v local image payload: missing data URL separator; upload the image to a URL or provide valid data:image/...;base64,...", nil)
		}

		header := strings.ToLower(strings.TrimSpace(trimmed[:comma]))
		if !strings.Contains(header, ";base64") {
			return internalerrors.New(internalerrors.ErrValidationFailed, "invalid i2v local image payload: expected data:image/...;base64,...; upload the image to a URL or provide valid data:image/...;base64,...", nil)
		}

		payload := stripBase64Whitespace(trimmed[comma+1:])
		if payload == "" {
			return internalerrors.New(internalerrors.ErrValidationFailed, "invalid i2v local image payload: base64 content is empty; upload the image to a URL or provide valid data:image/...;base64,...", nil)
		}

		if !isStrictStdBase64(payload) {
			return internalerrors.New(internalerrors.ErrValidationFailed, "invalid i2v local image payload: invalid base64 content; upload the image to a URL or provide valid data:image/...;base64,...", nil)
		}

		if estimatedBase64DecodedLength(payload) > maxVideoInlineImageBytes {
			return internalerrors.New(
				internalerrors.ErrValidationFailed,
				fmt.Sprintf("local i2v image payload is too large (max %d bytes after decode); upload the image to a URL or compress it before submit", maxVideoInlineImageBytes),
				nil,
			)
		}
	}

	return nil
}

func isStrictStdBase64(s string) bool {
	if s == "" {
		return false
	}

	padding := 0
	seenPad := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			if seenPad {
				return false
			}
		case c >= 'a' && c <= 'z':
			if seenPad {
				return false
			}
		case c >= '0' && c <= '9':
			if seenPad {
				return false
			}
		case c == '+' || c == '/':
			if seenPad {
				return false
			}
		case c == '=':
			seenPad = true
			padding++
			if padding > 2 {
				return false
			}
		default:
			return false
		}
	}

	return true
}

func estimatedBase64DecodedLength(payload string) int {
	n := len(payload)
	if n == 0 {
		return 0
	}

	padding := 0
	if payload[n-1] == '=' {
		padding++
		if n > 1 && payload[n-2] == '=' {
			padding++
		}
	}

	decoded := (n * 3 / 4) - padding
	if decoded < 0 {
		return 0
	}

	return decoded
}
