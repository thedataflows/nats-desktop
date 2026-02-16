package components

import (
	"image"
	"image/color"
	"io"
	"strings"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
	"github.com/thedataflows/nats-desktop/internal/utils"
)

// LabelPosition determines where the label is displayed relative to the input
type LabelPosition int

const (
	LabelPositionLeft LabelPosition = iota // Label displayed to the left of input
	LabelPositionTop                       // Label displayed above the input
)

type InputField struct {
	editor widget.Editor

	Icon               *widget.Icon
	iconClick          widget.Clickable
	IconPosition       IconPosition
	BorderColor        color.NRGBA
	BorderColorFocused color.NRGBA
	Placeholder        string
	Label              string
	Hint               string
	ErrorText          string

	SingleLine    bool
	Required      bool
	Disabled      bool
	ShowLabel     bool
	LabelPosition LabelPosition // Where to display the label (Top or Left)
	MinWidth      unit.Dp
	MaxWidth      unit.Dp
	LabelWidth    unit.Dp
	IconSize      unit.Dp
	IconColor     color.NRGBA
	cornerRadius  unit.Dp

	size image.Point

	onIconClick  func()
	onTextChange func(text string)
	onTab        func(gtx layout.Context, shift bool)
	onSubmit     func()

	changed bool
	focused bool

	menuContextArea component.ContextArea
	menu            component.MenuState
	menuInit        bool
	menuClickables  []*widget.Clickable
	showMenu        bool
	menuPos         image.Point
}

func NewInputField(placeholder string) *InputField {
	return &InputField{
		Placeholder:  placeholder,
		SingleLine:   true,
		MinWidth:     unit.Dp(200),
		LabelWidth:   unit.Dp(100),
		IconSize:     unit.Dp(18),
		cornerRadius: unit.Dp(8),
		menuContextArea: component.ContextArea{
			Activation:       pointer.ButtonSecondary,
			AbsolutePosition: false,
		},
	}
}

func NewLabeledInputField(label, placeholder string) *InputField {
	return NewLabeledInputFieldWithPosition(label, placeholder, LabelPositionLeft)
}

func NewLabeledInputFieldWithPosition(label, placeholder string, pos LabelPosition) *InputField {
	i := NewInputField(placeholder)
	i.Label = label
	i.ShowLabel = true
	i.LabelPosition = pos
	return i
}

func (i *InputField) GetText() string {
	return i.editor.Text()
}

func (i *InputField) SetText(text string) {
	i.editor.SetText(text)
}

func (i *InputField) Changed() bool {
	if i.changed {
		i.changed = false
		return true
	}
	return false
}

func (i *InputField) FocusTag() *widget.Editor {
	return &i.editor
}

func (i *InputField) SetIcon(icon *widget.Icon, position IconPosition) {
	i.Icon = icon
	i.IconPosition = position
}

func (i *InputField) SetSize(s image.Point) {
	i.size = s
}

func (i *InputField) SetMinWidth(width unit.Dp) {
	i.MinWidth = width
}

func (i *InputField) SetMaxWidth(width unit.Dp) {
	i.MaxWidth = width
}

func (i *InputField) SetLabelWidth(width unit.Dp) {
	i.LabelWidth = width
}

func (i *InputField) SetBorderColor(borderColor, focusedColor color.NRGBA) {
	i.BorderColor = borderColor
	i.BorderColorFocused = focusedColor
}

func (i *InputField) SetError(text string) {
	i.ErrorText = text
}

func (i *InputField) SetDisabled(disabled bool) {
	i.Disabled = disabled
}

func (i *InputField) SetRequired(required bool) {
	i.Required = required
}

func (i *InputField) SetLabelPosition(pos LabelPosition) {
	i.LabelPosition = pos
}

func (i *InputField) SetOnTextChange(f func(text string)) {
	i.onTextChange = f
}

func (i *InputField) SetOnIconClick(f func()) {
	i.onIconClick = f
}

func (i *InputField) SetOnTab(f func(gtx layout.Context, shift bool)) {
	i.onTab = f
}

func (i *InputField) SetOnSubmit(f func()) {
	i.onSubmit = f
}

func (i *InputField) SelectAll() {
	text := i.editor.Text()
	if len(text) > 0 {
		// Select all text - SetCaret takes rune offsets
		i.editor.SetCaret(0, i.editor.Len())
	}
}

func (i *InputField) GetSelectedText() string {
	return i.editor.SelectedText()
}

func (i *InputField) HasSelection() bool {
	start, end := i.editor.Selection()
	return start != end
}

func (i *InputField) Copy(gtx layout.Context) {
	if selected := i.GetSelectedText(); selected != "" {
		gtx.Execute(clipboard.WriteCmd{
			Type: "text/plain",
			Data: io.NopCloser(strings.NewReader(selected)),
		})
	}
}

func (i *InputField) Cut(gtx layout.Context) {
	// Use the editor's native SelectedText method
	selected := i.editor.SelectedText()

	// If no selection, select all text
	if selected == "" {
		i.SelectAll()
		selected = i.editor.SelectedText()
	}

	// Copy to clipboard
	if selected != "" {
		gtx.Execute(clipboard.WriteCmd{
			Type: "text/plain",
			Data: io.NopCloser(strings.NewReader(selected)),
		})

		// Delete the selected text using the editor's native method
		start, end := i.editor.Selection()
		if start > end {
			start, end = end, start
		}
		if start < end {
			text := i.editor.Text()
			newText := text[:start] + text[end:]
			i.editor.SetText(newText)
			i.editor.SetCaret(start, start)
			i.changed = true
			if i.onTextChange != nil {
				i.onTextChange(newText)
			}
		}
	}
}

func (i *InputField) Paste(gtx layout.Context) {
	gtx.Execute(clipboard.ReadCmd{Tag: &i.editor})
}

func (i *InputField) Delete() {
	start, end := i.editor.Selection()
	if start > end {
		start, end = end, start
	}
	if start != end {
		text := i.editor.Text()
		newText := text[:start] + text[end:]
		i.editor.SetText(newText)
		i.editor.SetCaret(start, start)
		i.changed = true
		if i.onTextChange != nil {
			i.onTextChange(newText)
		}
	}
}

func (i *InputField) handleClipboardRead(gtx layout.Context) {
	for {
		event, ok := gtx.Event(transfer.TargetFilter{Target: &i.editor, Type: "text/plain"})
		if !ok {
			break
		}
		switch e := event.(type) {
		case transfer.DataEvent:
			if e.Type == "text/plain" {
				reader := e.Open()
				if reader != nil {
					buf := new(strings.Builder)
					if _, err := io.Copy(buf, reader); err == nil {
						start, end := i.editor.CaretPos()
						if start > end {
							start, end = end, start
						}
						text := i.editor.Text()
						newText := text[:start] + buf.String() + text[end:]
						i.editor.SetText(newText)
						newPos := start + len(buf.String())
						i.editor.SetCaret(newPos, newPos)
						i.changed = true
						if i.onTextChange != nil {
							i.onTextChange(newText)
						}
					}
					reader.Close()
				}
			}
		}
	}
}

func (i *InputField) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	i.handleClipboardRead(gtx)
	i.handleKeyboardEvents(gtx)
	i.handleEditorUpdates(gtx)
	i.handlePointerEvents(gtx)

	i.focused = gtx.Source.Focused(&i.editor)

	if !i.menuInit {
		i.menuInit = true
		menuLabels := []string{"Cut", "Copy", "Paste", "Delete", "Select All"}
		i.menuClickables = make([]*widget.Clickable, len(menuLabels))
		i.menu.Options = make([]func(gtx layout.Context) layout.Dimensions, len(menuLabels))
		for idx, label := range menuLabels {
			i.menuClickables[idx] = new(widget.Clickable)
			lbl := label
			clickable := i.menuClickables[idx]
			i.menu.Options[idx] = func(gtx layout.Context) layout.Dimensions {
				return component.MenuItem(th.Material(), clickable, lbl).Layout(gtx)
			}
		}
	}

	for idx, clickable := range i.menuClickables {
		if clickable.Clicked(gtx) {
			// Dismiss the context menu
			i.menuContextArea.Dismiss()
			switch idx {
			case 0:
				i.Cut(gtx)
			case 1:
				i.Copy(gtx)
			case 2:
				i.Paste(gtx)
			case 3:
				i.Delete()
			case 4:
				i.SelectAll()
			}
		}
	}

	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if i.ShowLabel {
				// Render label based on LabelPosition setting
				if i.LabelPosition == LabelPositionTop {
					// Label above the input
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
							label := material.Label(th.Material(), unit.Sp(13), i.Label)
							if i.Required {
								label.Text += " *"
								label.Font.Weight = font.SemiBold
								label.Color = theme.LightRed
							} else {
								label.Font.Weight = font.Medium
								label.Color = th.TextColor
							}
							return layout.Inset{Bottom: unit.Dp(6)}.Layout(cgtx, label.Layout)
						}),
						layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
							if i.MinWidth > 0 {
								cgtx.Constraints.Min.X = cgtx.Dp(i.MinWidth)
							}
							if i.MaxWidth > 0 {
								cgtx.Constraints.Max.X = cgtx.Dp(i.MaxWidth)
								if cgtx.Constraints.Max.X < cgtx.Constraints.Min.X {
									cgtx.Constraints.Min.X = cgtx.Constraints.Max.X
								}
							}
							dims := i.layoutContent(cgtx, th)
							if i.focused {
								DrawFocusRing(cgtx, th.BorderColorFocused, dims.Size, cgtx.Dp(i.cornerRadius))
							}
							return dims
						}),
					)
				} else {
					// Label to the left of input (default)
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
							cgtx.Constraints.Min.X = cgtx.Dp(i.LabelWidth)
							label := material.Label(th.Material(), unit.Sp(13), i.Label)
							if i.Required {
								label.Text += " *"
								label.Font.Weight = font.SemiBold
								label.Color = theme.LightRed
							} else {
								label.Font.Weight = font.Medium
								label.Color = th.TextColor
							}
							return label.Layout(cgtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
						layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
							if i.MinWidth > 0 {
								cgtx.Constraints.Min.X = cgtx.Dp(i.MinWidth)
							}
							if i.MaxWidth > 0 {
								cgtx.Constraints.Max.X = cgtx.Dp(i.MaxWidth)
								if cgtx.Constraints.Max.X < cgtx.Constraints.Min.X {
									cgtx.Constraints.Min.X = cgtx.Constraints.Max.X
								}
							}
							dims := i.layoutContent(cgtx, th)
							if i.focused {
								DrawFocusRing(cgtx, th.BorderColorFocused, dims.Size, cgtx.Dp(i.cornerRadius))
							}
							return dims
						}),
					)
				}
			}
			dims := i.layoutContent(gtx, th)
			if i.focused {
				DrawFocusRing(gtx, th.BorderColorFocused, dims.Size, gtx.Dp(i.cornerRadius))
			}
			return dims
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return i.menuContextArea.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// Position menu at captured click location
				defer op.Offset(i.menuPos).Push(gtx.Ops).Pop()
				gtx.Constraints.Min = image.Point{}
				m := component.Menu(th.Material(), &i.menu)
				m.SurfaceStyle.Fill = th.MenuBgColor
				return m.Layout(gtx)
			})
		}),
	)
}

func (i *InputField) handleKeyboardEvents(gtx layout.Context) {
	for {
		event, ok := gtx.Event(
			key.Filter{Focus: &i.editor, Name: key.NameEscape},
			key.Filter{Focus: &i.editor, Name: key.NameReturn},
			key.Filter{Focus: &i.editor, Name: key.NameDeleteBackward, Required: key.ModShortcut},
		)
		if !ok {
			break
		}
		if ev, ok := event.(key.Event); ok && ev.State == key.Press {
			switch ev.Name {
			case key.NameEscape:
				gtx.Execute(key.FocusCmd{Tag: nil})
			case key.NameReturn:
				if i.SingleLine && i.onSubmit != nil {
					i.onSubmit()
				}
			case key.NameDeleteBackward:
				if ev.Modifiers.Contain(key.ModShortcut) {
					utils.DeleteWordBackward(&i.editor)
				}
			}
		}
	}
	if i.onTab != nil {
		for {
			ev, ok := gtx.Event(key.Filter{Focus: &i.editor, Name: key.NameTab, Optional: key.ModShift})
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press {
				i.onTab(gtx, e.Modifiers.Contain(key.ModShift))
			}
		}
	}
}

func (i *InputField) handleEditorUpdates(gtx layout.Context) {
	for {
		event, ok := i.editor.Update(gtx)
		if !ok {
			break
		}
		switch event.(type) {
		case widget.ChangeEvent, widget.SubmitEvent:
			i.changed = true
			if i.onTextChange != nil {
				i.onTextChange(i.editor.Text())
			}
			gtx.Execute(op.InvalidateCmd{})
		}
	}
}

func (i *InputField) handlePointerEvents(gtx layout.Context) {
	for {
		event, ok := gtx.Event(
			pointer.Filter{
				Target: &i.editor,
				Kinds:  pointer.Press,
			},
		)
		if !ok {
			break
		}
		switch e := event.(type) {
		case pointer.Event:
			if e.Kind == pointer.Press && e.Buttons.Contain(pointer.ButtonSecondary) {
				// Capture the click position within the input field
				// This is relative to the editor widget
				i.menuPos = image.Pt(int(e.Position.X), int(e.Position.Y))
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}
}

func (i *InputField) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	borderColor := i.BorderColor
	if borderColor == (color.NRGBA{}) {
		borderColor = th.BorderColor
	}
	if gtx.Source.Focused(&i.editor) {
		if i.BorderColorFocused == (color.NRGBA{}) {
			borderColor = th.BorderColorFocused
		} else {
			borderColor = i.BorderColorFocused
		}
	}
	if i.ErrorText != "" {
		borderColor = theme.LightRed
	}
	if i.Disabled {
		borderColor = WithAlpha(borderColor, 100)
	}

	border := widget.Border{
		Color:        borderColor,
		Width:        unit.Dp(1),
		CornerRadius: i.cornerRadius,
	}

	editor := material.Editor(th.Material(), &i.editor, i.Placeholder)
	editor.TextSize = unit.Sp(14)
	editor.Color = th.TextColor
	editor.HintColor = WithAlpha(th.TextColor, 140)
	if i.Disabled {
		editor.Color = WithAlpha(th.TextColor, 140)
	}

	return border.Layout(gtx, func(ccgtx layout.Context) layout.Dimensions {
		if i.size.X == 0 {
			i.size.X = ccgtx.Constraints.Min.X
		}
		ccgtx.Constraints.Min = i.size

		dims := layout.Inset{
			Top:    unit.Dp(8),
			Bottom: unit.Dp(8),
			Left:   unit.Dp(12),
			Right:  unit.Dp(12),
		}.Layout(ccgtx, func(cccgtx layout.Context) layout.Dimensions {
			iconLayout := layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
				if i.Icon == nil {
					return layout.Dimensions{}
				}
				if i.onIconClick != nil && i.iconClick.Clicked(c4gtx) {
					i.onIconClick()
				}
				if i.IconColor == (color.NRGBA{}) {
					i.IconColor = WithAlpha(th.TextColor, 160)
				}
				return layout.Inset{Right: unit.Dp(8)}.Layout(c4gtx, func(c5gtx layout.Context) layout.Dimensions {
					c5gtx.Constraints.Min.X = c5gtx.Dp(i.IconSize)
					c5gtx.Constraints.Min.Y = c5gtx.Dp(i.IconSize)
					return i.Icon.Layout(c5gtx, i.IconColor)
				})
			})

			editorLayout := layout.Flexed(1, func(c4gtx layout.Context) layout.Dimensions {
				return editor.Layout(c4gtx)
			})

			widgets := []layout.FlexChild{editorLayout}
			if i.Icon != nil {
				widgets = []layout.FlexChild{iconLayout, editorLayout}
			}

			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(cccgtx, widgets...)
		})
		return dims
	})
}
