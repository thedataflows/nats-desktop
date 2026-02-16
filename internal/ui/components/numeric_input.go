package components

import (
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

// NumericInputField is an input field that only accepts numeric values
type NumericInputField struct {
	*InputField
	MinValue *int
	MaxValue *int
}

// NewNumericInputField creates a new numeric input field
func NewNumericInputField(placeholder string) *NumericInputField {
	input := NewInputField(placeholder)
	return &NumericInputField{
		InputField: input,
	}
}

// NewLabeledNumericInputField creates a new numeric input field with a label on the left
func NewLabeledNumericInputField(label, placeholder string) *NumericInputField {
	input := NewLabeledInputField(label, placeholder)
	return &NumericInputField{
		InputField: input,
	}
}

// NewLabeledNumericInputFieldWithPosition creates a new numeric input field with a label in the specified position
func NewLabeledNumericInputFieldWithPosition(label, placeholder string, pos LabelPosition) *NumericInputField {
	input := NewLabeledInputFieldWithPosition(label, placeholder, pos)
	return &NumericInputField{
		InputField: input,
	}
}

// GetValue returns the numeric value, or 0 if empty/invalid
func (n *NumericInputField) GetValue() int {
	text := n.GetText()
	if text == "" {
		return 0
	}
	val, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return val
}

// SetValue sets the numeric value
func (n *NumericInputField) SetValue(val int) {
	n.SetText(strconv.Itoa(val))
}

// Layout renders the numeric input field with validation
func (n *NumericInputField) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	// Validate on change - filter non-numeric characters
	text := n.GetText()
	if text != "" {
		// Remove any non-numeric characters
		filtered := filterNumeric(text)
		if filtered != text {
			n.SetText(filtered)
		}

		// Validate range if set
		if val, err := strconv.Atoi(filtered); err == nil {
			if n.MinValue != nil && val < *n.MinValue {
				n.SetValue(*n.MinValue)
			}
			if n.MaxValue != nil && val > *n.MaxValue {
				n.SetValue(*n.MaxValue)
			}
		}
	}

	return n.InputField.Layout(gtx, th)
}

// filterNumeric removes all non-numeric characters from a string
func filterNumeric(s string) string {
	var result strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// FocusTag returns the focus tag for the input field
func (n *NumericInputField) FocusTag() any {
	return n.InputField.FocusTag()
}

// SetText sets the text value (converts to string)
func (n *NumericInputField) SetText(text string) {
	n.InputField.SetText(text)
}

// SetOnTextChange sets the callback for text changes
func (n *NumericInputField) SetOnTextChange(fn func(text string)) {
	n.InputField.SetOnTextChange(fn)
}

// SetMinWidth sets the minimum width
func (n *NumericInputField) SetMinWidth(width unit.Dp) {
	n.InputField.SetMinWidth(width)
}

// SetMinValue sets the minimum allowed value
func (n *NumericInputField) SetMinValue(val int) {
	n.MinValue = &val
}

// SetMaxValue sets the maximum allowed value
func (n *NumericInputField) SetMaxValue(val int) {
	n.MaxValue = &val
}

// SetRequired sets whether the field is required
func (n *NumericInputField) SetRequired(required bool) {
	n.InputField.SetRequired(required)
}

// SetDisabled sets whether the field is disabled
func (n *NumericInputField) SetDisabled(disabled bool) {
	n.InputField.SetDisabled(disabled)
}

// SetError sets the error text
func (n *NumericInputField) SetError(err string) {
	n.InputField.SetError(err)
}

// Changed returns whether the value has changed
func (n *NumericInputField) Changed() bool {
	return n.InputField.Changed()
}
