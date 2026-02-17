package components

import (
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
)

func CopyToClipboard(gtx layout.Context, text string) {
	gtx.Execute(clipboard.WriteCmd{
		Type: "text/plain",
		Data: io.NopCloser(strings.NewReader(text)),
	})
}

func TruncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}
