package utils

import (
	"testing"
)

func TestDetectPayloadFormat(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{"Empty", []byte(""), "Empty"},
		{"JSON Object", []byte(`{"a": 1}`), "JSON"},
		{"JSON Array", []byte(`[1, 2, 3]`), "JSON"},
		{"XML", []byte(`<a>b</a>`), "XML"},
		{"Text", []byte("Hello World"), "Text"},
		{"Binary", []byte{0x00, 0xFF, 0x01}, "Unknown format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectPayloadFormat(tt.data); got != tt.expected {
				t.Errorf("%s: DetectPayloadFormat() = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}
