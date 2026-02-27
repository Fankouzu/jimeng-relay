package jimeng

import (
	"fmt"
	"strconv"
	"strings"
)

// ToString converts an interface to a string.
// It handles nil, string, float64, int, and other types using fmt.Sprintf.
func ToString(v interface{}) string {
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

// ToInt converts an interface to an int.
// It handles nil, int, int32, int64, float32, float64, and string.
func ToInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	case float32:
		return int(val)
	case float64:
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

// ToStringSlice converts an interface to a string slice.
// It handles nil, []string, and []interface{}.
func ToStringSlice(v interface{}) []string {
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

// CleanStringSlice trims spaces from each string in the slice and removes empty strings.
func CleanStringSlice(v []string) []string {
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


// ParseInt parses a string to an int.
func ParseInt(s string) (int, error) {
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

// ParseFloat parses a string to a float64.
// It supports fractions like "16/9".
func ParseFloat(s string) (float64, error) {
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
