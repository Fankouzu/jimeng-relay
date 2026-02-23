package output

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jimeng-relay/client/internal/jimeng"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

type Formatter struct {
	Format Format
}

func NewFormatter(format Format) *Formatter {
	return &Formatter{Format: format}
}

func (f *Formatter) FormatSubmitResponse(resp *jimeng.SubmitResponse) (string, error) {
	format := FormatText
	if f != nil && f.Format != "" {
		format = f.Format
	}

	switch format {
	case FormatJSON:
		b, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	case FormatText:
		if resp == nil {
			return "", nil
		}
		return fmt.Sprintf("TaskID=%s", resp.TaskID), nil
	default:
		return "", fmt.Errorf("unsupported format: %q", format)
	}
}

func (f *Formatter) FormatGetResultResponse(resp *jimeng.GetResultResponse) (string, error) {
	format := FormatText
	if f != nil && f.Format != "" {
		format = f.Format
	}

	switch format {
	case FormatJSON:
		b, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	case FormatText:
		if resp == nil {
			return "", nil
		}
		return fmt.Sprintf(
			"Status=%s ImageURLs=%s",
			resp.Status,
			strings.Join(resp.ImageURLs, ","),
		), nil
	default:
		return "", fmt.Errorf("unsupported format: %q", format)
	}
}
