package utils

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"unicode/utf8"
)

func DetectPayloadFormat(data []byte) string {
	if len(data) == 0 {
		return "Empty"
	}

	// Try JSON
	var j interface{}
	if err := json.Unmarshal(data, &j); err == nil {
		return "JSON"
	}

	// Try XML
	var x interface{}
	if err := xml.Unmarshal(data, &x); err == nil {
		return "XML"
	}

	// Try UTF-8 String
	if utf8.Valid(data) {
		return "Text"
	}

	return "Unknown format"
}

func FormatNumber(num int64) string {
	if num >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(num)/1000000)
	}
	if num >= 1000 {
		return fmt.Sprintf("%.1fK", float64(num)/1000)
	}
	return fmt.Sprintf("%d", num)
}

func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes >= unit*unit*unit {
		return fmt.Sprintf("%.2f GB", float64(bytes)/(unit*unit*unit))
	}
	if bytes >= unit*unit {
		return fmt.Sprintf("%.2f MB", float64(bytes)/(unit*unit))
	}
	if bytes >= unit {
		return fmt.Sprintf("%.2f KB", float64(bytes)/unit)
	}
	return fmt.Sprintf("%d B", bytes)
}

func Strikethrough(s string) string {
	var result string
	for _, r := range s {
		result += string(r) + "\u0336"
	}
	return result
}
