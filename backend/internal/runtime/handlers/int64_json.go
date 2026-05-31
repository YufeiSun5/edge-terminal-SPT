package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type flexibleInt64 int64

func (v *flexibleInt64) UnmarshalJSON(raw []byte) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		*v = 0
		return nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			*v = 0
			return nil
		}
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid int64 string %q", text)
		}
		*v = flexibleInt64(parsed)
		return nil
	}
	parsed, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid int64 value %q", trimmed)
	}
	*v = flexibleInt64(parsed)
	return nil
}

func (v flexibleInt64) Int64() int64 {
	return int64(v)
}

type flexibleInt64List []int64

func (v *flexibleInt64List) UnmarshalJSON(raw []byte) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		*v = nil
		return nil
	}
	var items []flexibleInt64
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("invalid int64 list: %w", err)
	}
	out := make([]int64, 0, len(items))
	for _, item := range items {
		id := item.Int64()
		if id != 0 {
			out = append(out, id)
		}
	}
	*v = out
	return nil
}

func (v flexibleInt64List) Int64s() []int64 {
	return []int64(v)
}
