package utils

import (
	"unicode"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type EditorStyle struct {
	Editor   *widget.Editor
	Hint     string
	TextSize unit.Sp
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func DeleteWordBackward(ed *widget.Editor) {
	start, end := ed.Selection()
	if start != end {
		ed.Delete(1)
		return
	}

	if start == 0 {
		return
	}

	text := ed.Text()
	runes := []rune(text)
	// Caret position in runes is not necessarily start.
	// ed.Selection() returns byte offsets.

	// Better use runes directly if possible, but Gio Editor works with rune indices for Selection?
	// Let's check Gio docs or common usage.
	// Actually ed.Selection() returns rune offsets in recent Gio versions.

	pos := start
	if pos <= 0 {
		return
	}

	i := pos

	// If we are at whitespace, delete all whitespace first
	for i > 0 && unicode.IsSpace(runes[i-1]) {
		i--
	}

	if i < pos {
		ed.Delete(i - pos)
		return
	}

	// If we are at a word character, delete until non-word
	if i > 0 && isWordChar(runes[i-1]) {
		for i > 0 && isWordChar(runes[i-1]) {
			i--
		}
	} else if i > 0 {
		// If we are at a non-word character (like . or /), delete all consecutive non-word chars
		for i > 0 && !isWordChar(runes[i-1]) && !unicode.IsSpace(runes[i-1]) {
			i--
		}
	}

	ed.Delete(i - pos)
}

func NewEditor(th *material.Theme, editor *widget.Editor, hint string) EditorStyle {
	return EditorStyle{
		Editor:   editor,
		Hint:     hint,
		TextSize: unit.Sp(14),
	}
}

func (e EditorStyle) Layout(gtx layout.Context, th *material.Theme) layout.Dimensions {
	editor := material.Editor(th, e.Editor, e.Hint)
	editor.TextSize = e.TextSize
	return editor.Layout(gtx)
}
