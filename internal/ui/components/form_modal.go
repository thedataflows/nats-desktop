package components

import (
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
	"github.com/thedataflows/nats-desktop/internal/utils"
)

type FormField struct {
	Label    string
	Hint     string
	Editor   *widget.Editor
	Password bool
	// Input is an alias for Editor for backward compatibility
	Input *widget.Editor
	// Required marks if the field is required
	Required bool
	// InputField is an alternative to Editor/Input for using the InputField component
	InputField *InputField
}

type FormModal struct {
	Title     string
	Fields    []FormField
	SaveBtn   *widget.Clickable
	CancelBtn *widget.Clickable
	OnSave    func() bool
	OnCancel  func()
	OnClose   func()
	Visible   bool
	Modal     Modal

	ReturnFocus event.Tag

	// CustomContent allows providing a custom layout function instead of using Fields
	// If set, this will be used instead of rendering Fields
	CustomContent func(gtx layout.Context, th *theme.Theme) layout.Dimensions

	// CustomFocusTags provides focus tags for tab navigation when using CustomContent
	// These tags will be cycled through when pressing Tab
	CustomFocusTags []event.Tag

	// CustomFocusTagsFunc provides focus tags dynamically for tab navigation
	// This is useful when the set of focusable widgets changes based on state
	// (e.g., collapsed ExpandableSections). If set, this takes precedence over CustomFocusTags.
	CustomFocusTagsFunc func() []event.Tag

	// MaxHeight constrains the modal height (0 = no constraint)
	MaxHeight unit.Dp

	// MaxWidth constrains the modal width (0 = uses default 600)
	MaxWidth unit.Dp

	// HideSaveButton hides the save button (for read-only modals like info dialogs)
	HideSaveButton bool

	// BlockEvents prevents the modal from processing keyboard events
	// Useful when a child modal is open on top of this one
	BlockEvents bool

	// OnEnter is called when Enter/Return is pressed (for read-only modals)
	OnEnter func()

	// SaveButtonText overrides the default "Save" button text
	SaveButtonText string
}

func NewFormModal(title string) *FormModal {
	return &FormModal{
		Title:     title,
		SaveBtn:   &widget.Clickable{},
		CancelBtn: &widget.Clickable{},
	}
}

func (f *FormModal) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if !f.Visible {
		return layout.Dimensions{}
	}

	if f.Modal.Backdrop == nil {
		f.Modal.Backdrop = &widget.Clickable{}
	}

	// Capture all key events within the modal area
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, f)
	area.Pop()

	// Handle Keyboard Events (only if not blocked)
	if !f.BlockEvents {
		// Handle Escape globally (closes modal)
		for {
			ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				f.closeModal(gtx)
				if f.OnCancel != nil {
					f.OnCancel()
				}
			}
		}

		// Handle Tab globally (navigates between fields)
		for {
			ev, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				f.handleTab(gtx, ke.Modifiers.Contain(key.ModShift))
			}
		}

		// Handle Space/Enter/Return only when Save or Cancel buttons are focused
		// (not globally, so child widgets like checkboxes can receive these events)
		for {
			ev, ok := gtx.Event(
				key.Filter{Focus: f.SaveBtn, Name: key.NameReturn},
				key.Filter{Focus: f.SaveBtn, Name: key.NameEnter},
				key.Filter{Focus: f.SaveBtn, Name: key.NameSpace},
				key.Filter{Focus: f.CancelBtn, Name: key.NameReturn},
				key.Filter{Focus: f.CancelBtn, Name: key.NameEnter},
				key.Filter{Focus: f.CancelBtn, Name: key.NameSpace},
			)
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				if gtx.Source.Focused(f.SaveBtn) {
					success := true
					if f.OnSave != nil {
						success = f.OnSave()
					}
					if success {
						f.closeModal(gtx)
					}
				} else if gtx.Source.Focused(f.CancelBtn) {
					if f.OnCancel != nil {
						f.OnCancel()
					}
					f.closeModal(gtx)
				}
			}
		}
	}

	// Handle backdrop clicks
	for f.Modal.Backdrop.Clicked(gtx) {
		f.closeModal(gtx)
		if f.OnCancel != nil {
			f.OnCancel()
		}
	}

	// Initial focus if nothing is focused
	// When using CustomContent, we skip auto-focus and let the custom content handle it
	if f.CustomContent == nil {
		anyFocused := false
		for i := range f.Fields {
			field := &f.Fields[i]
			if field.InputField != nil {
				if gtx.Source.Focused(field.InputField.FocusTag()) {
					anyFocused = true
					break
				}
			} else {
				editor := field.Editor
				if editor == nil {
					editor = field.Input
				}
				if editor != nil && gtx.Source.Focused(editor) {
					anyFocused = true
					break
				}
			}
		}
		if !anyFocused && !gtx.Source.Focused(f.SaveBtn) && !gtx.Source.Focused(f.CancelBtn) {
			if len(f.Fields) > 0 {
				field := &f.Fields[0]
				if field.InputField != nil {
					gtx.Execute(key.FocusCmd{Tag: field.InputField.FocusTag()})
				} else {
					editor := field.Editor
					if editor == nil {
						editor = field.Input
					}
					if editor != nil {
						gtx.Execute(key.FocusCmd{Tag: editor})
					} else {
						gtx.Execute(key.FocusCmd{Tag: f.SaveBtn})
					}
				}
			} else {
				gtx.Execute(key.FocusCmd{Tag: f.SaveBtn})
			}
		}
	}

	content := func(cgtx layout.Context) layout.Dimensions {
		var formContent layout.Widget
		if f.CustomContent != nil {
			formContent = func(cgtx layout.Context) layout.Dimensions {
				if f.MaxHeight > 0 {
					maxHeightPx := cgtx.Dp(f.MaxHeight)
					if cgtx.Constraints.Max.Y > maxHeightPx {
						cgtx.Constraints.Max.Y = maxHeightPx
					}
				}
				return f.CustomContent(cgtx, th)
			}
		} else {
			formContent = func(cgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(cgtx, f.layoutFields(cgtx, th)...)
			}
		}

		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return formContent(cgtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceStart}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						btn := SecondaryButton(th, f.CancelBtn, nil, 0, "Close")
						for f.CancelBtn.Clicked(cccgtx) {
							if f.OnCancel != nil {
								f.OnCancel()
							}
							f.closeModal(cccgtx)
						}
						return btn.Layout(cccgtx, th)
					}),
					func() layout.FlexChild {
						if f.HideSaveButton {
							return layout.Rigid(layout.Spacer{Width: unit.Dp(0)}.Layout)
						}
						return layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout)
					}(),
					func() layout.FlexChild {
						if f.HideSaveButton {
							return layout.Rigid(layout.Spacer{Width: unit.Dp(0)}.Layout)
						}
						return layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							saveText := f.SaveButtonText
							if saveText == "" {
								saveText = "Save"
							}
							btn := Button(th, f.SaveBtn, nil, 0, saveText)
							for f.SaveBtn.Clicked(cccgtx) {
								success := true
								if f.OnSave != nil {
									success = f.OnSave()
								}
								if success {
									f.closeModal(cccgtx)
								}
							}
							return btn.Layout(cccgtx, th)
						})
					}(),
				)
			}),
		)
	}

	// Pass MaxWidth to the underlying Modal
	f.Modal.MaxWidth = f.MaxWidth

	return f.Modal.Layout(gtx, th, f.Title, content)
}

func (f *FormModal) layoutFields(gtx layout.Context, th *theme.Theme) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(f.Fields)*2)
	for i := range f.Fields {
		field := &f.Fields[i]
		children = append(children, layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					lbl := material.Label(th.Material(), unit.Sp(14), field.Label)
					lbl.Color = th.SecondaryTextColor
					return layout.Inset{Bottom: unit.Dp(4)}.Layout(ccgtx, lbl.Layout)
				}),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					// Use InputField if provided, otherwise fall back to Editor/Input
					if field.InputField != nil {
						return field.InputField.Layout(ccgtx, th)
					}

					editor := field.Editor
					if editor == nil {
						editor = field.Input
					}
					if editor == nil {
						return layout.Dimensions{}
					}

					editor.SingleLine = true
					for {
						e, ok := ccgtx.Event(
							key.Filter{Focus: editor, Name: key.NameReturn},
							key.Filter{Focus: editor, Name: key.NameDeleteBackward, Required: key.ModShortcut},
						)
						if !ok {
							break
						}
						if ev, ok := e.(key.Event); ok {
							if ev.Name == key.NameReturn {
								// Swallow Enter
							}
							if ev.Name == key.NameDeleteBackward && ev.Modifiers.Contain(key.ModShortcut) {
								utils.DeleteWordBackward(editor)
							}
						}
					}

					ccgtx.Constraints.Min.X = ccgtx.Constraints.Max.X
					dims := widget.Border{
						Color:        th.BorderColor,
						Width:        unit.Dp(1),
						CornerRadius: unit.Dp(8),
					}.Layout(ccgtx, func(cccgtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(8)).Layout(cccgtx, func(ccccgtx layout.Context) layout.Dimensions {
							ed := material.Editor(th.Material(), editor, field.Hint)
							if field.Password {
								// Mask with *
								editor.Mask = '*'
							}
							return ed.Layout(ccccgtx)
						})
					})

					if ccgtx.Source.Focused(editor) {
						DrawFocusRing(gtx, th.BorderColorFocused, dims.Size, gtx.Dp(unit.Dp(8)))
					}

					return dims
				}),
			)
		}))
		if i < len(f.Fields)-1 {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout))
		}
	}
	return children
}

func (f *FormModal) Show() {
	f.Visible = true
}

func (f *FormModal) Hide() {
	f.Visible = false
}

func (f *FormModal) closeModal(gtx layout.Context) {
	f.Visible = false
	if f.ReturnFocus != nil {
		gtx.Execute(key.FocusCmd{Tag: f.ReturnFocus})
	}
	if f.OnClose != nil {
		f.OnClose()
	}
	gtx.Execute(op.InvalidateCmd{})
}

// HandleTabNavigation handles tab/shift+tab navigation within the modal
// This can be called from child widgets that need to trigger tab navigation
func (f *FormModal) HandleTabNavigation(gtx layout.Context, shift bool) {
	f.handleTab(gtx, shift)
}

func (f *FormModal) handleTab(gtx layout.Context, shift bool) {
	var tags []event.Tag

	// If CustomFocusTagsFunc is provided, use it for dynamic focus tags
	if f.CustomFocusTagsFunc != nil {
		tags = append(tags, f.CustomFocusTagsFunc()...)
	} else if len(f.CustomFocusTags) > 0 {
		// If CustomFocusTags is provided, use those for tab navigation
		tags = append(tags, f.CustomFocusTags...)
	} else {
		// Otherwise, use fields from f.Fields
		for i := range f.Fields {
			field := &f.Fields[i]
			if field.InputField != nil {
				tags = append(tags, field.InputField.FocusTag())
			} else {
				editor := field.Editor
				if editor == nil {
					editor = field.Input
				}
				if editor != nil {
					tags = append(tags, editor)
				}
			}
		}
	}
	tags = append(tags, f.CancelBtn, f.SaveBtn)

	if len(tags) == 0 {
		return
	}

	curIdx := -1
	for i, tag := range tags {
		if gtx.Source.Focused(tag) {
			curIdx = i
			break
		}
	}

	if curIdx == -1 {
		gtx.Execute(key.FocusCmd{Tag: tags[0]})
		return
	}

	var nextIdx int
	if shift {
		nextIdx = (curIdx - 1 + len(tags)) % len(tags)
	} else {
		nextIdx = (curIdx + 1) % len(tags)
	}
	gtx.Execute(key.FocusCmd{Tag: tags[nextIdx]})
}

// FormModalStyle is a backward-compatible wrapper for the widgets.FormModal API
// Deprecated: Use FormModal instead
type FormModalStyle struct {
	Modal ModalStyle
	Open  bool

	Fields    []FormField
	SaveBtn   *widget.Clickable
	CancelBtn *widget.Clickable

	OnSave   func()
	OnCancel func()
}

// FormModalFunc creates a new FormModalStyle (backward-compatible with widgets.FormModal)
// Deprecated: Use NewFormModal instead
func FormModalFunc(th *theme.Theme, title string, fields []FormField, saveBtn, cancelBtn *widget.Clickable) FormModalStyle {
	return FormModalStyle{
		Modal: ModalStyle{
			Title:        title,
			Message:      "",
			TitleColor:   nil,
			MessageColor: nil,
			Inset:        layout.UniformInset(unit.Dp(24)),
			MaxWidth:     unit.Dp(500),
			ShowIcon:     false,
			Backdrop:     &widget.Clickable{},
			ConfirmBtn:   saveBtn,
			CancelBtn:    cancelBtn,
		},
		Fields:    fields,
		SaveBtn:   saveBtn,
		CancelBtn: cancelBtn,
		OnSave:    func() {},
		OnCancel:  func() {},
	}
}

func (f *FormModalStyle) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if !f.Open {
		return layout.Dimensions{}
	}

	f.handleTab(gtx)

	if f.CancelBtn != nil && f.CancelBtn.Clicked(gtx) {
		if f.OnCancel != nil {
			f.OnCancel()
		}
		f.Hide()
	}
	if f.SaveBtn != nil && f.SaveBtn.Clicked(gtx) {
		if f.OnSave != nil {
			f.OnSave()
		}
		f.Hide()
	}

	content := func(gtx layout.Context) layout.Dimensions {
		return f.Modal.Inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					title := material.Label(th.Material(), unit.Sp(20), f.Modal.Title)
					title.Color = th.TextColor
					return title.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return f.layoutFields(gtx, th)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if f.CancelBtn != nil {
								btn := material.Button(th.Material(), f.CancelBtn, "Cancel")
								btn.Background = th.Palette.ContrastBg
								dims := btn.Layout(gtx)
								if gtx.Focused(f.CancelBtn) {
									DrawFocusRing(gtx, th.BorderColorFocused, dims.Size, gtx.Dp(unit.Dp(4)))
								}
								return dims
							}
							return layout.Dimensions{}
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if f.SaveBtn != nil {
								btn := material.Button(th.Material(), f.SaveBtn, "Save")
								btn.Background = th.ActionButtonBgColor
								dims := btn.Layout(gtx)
								if gtx.Focused(f.SaveBtn) {
									DrawFocusRing(gtx, th.BorderColorFocused, dims.Size, gtx.Dp(unit.Dp(4)))
								}
								return dims
							}
							return layout.Dimensions{}
						}),
					)
				}),
			)
		})
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = gtx.Dp(f.Modal.MaxWidth)

		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				backdrop := MulAlpha(th.Palette.Fg, 200)
				paint.FillShape(gtx.Ops, backdrop, clip.Rect{
					Max: gtx.Constraints.Max,
				}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
			layout.Stacked(content),
		)
	})
}

func (f *FormModalStyle) layoutFields(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	children := make([]layout.FlexChild, len(f.Fields))
	for i, field := range f.Fields {
		field := field
		children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Label(th.Material(), unit.Sp(13), field.Label)
						label.Color = th.TextColor
						if field.Password {
							label.Text += " *"
						}
						label.Font.Weight = font.SemiBold
						return label.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// Use InputField if provided, otherwise fall back to Editor/Input
						if field.InputField != nil {
							return field.InputField.Layout(gtx, th)
						}

						// Handle backward compatibility: Input is an alias for Editor
						editor := field.Editor
						if editor == nil && field.Input != nil {
							editor = field.Input
						}
						if editor == nil {
							return layout.Dimensions{}
						}

						editor.SingleLine = true
						for {
							event, ok := gtx.Event(
								key.Filter{Focus: editor, Name: key.NameEscape},
								key.Filter{Focus: editor, Name: key.NameReturn},
								key.Filter{Focus: editor, Name: key.NameDeleteBackward, Required: key.ModShortcut},
							)
							if !ok {
								break
							}

							if ev, ok := event.(key.Event); ok {
								if ev.Name == key.NameEscape {
									gtx.Execute(key.FocusCmd{Tag: nil})
								}
								if ev.Name == key.NameReturn {
									// Swallow enter key to prevent newline
								}
								if ev.Name == key.NameDeleteBackward && ev.Modifiers.Contain(key.ModShortcut) {
									utils.DeleteWordBackward(editor)
								}
							}
						}

						return widget.Border{
							Color:        th.BorderColor,
							Width:        unit.Dp(1),
							CornerRadius: unit.Dp(8),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								editorWidget := material.Editor(th.Material(), editor, field.Hint)
								editorWidget.TextSize = unit.Sp(14)
								editorWidget.Color = th.TextColor
								editorWidget.HintColor = MulAlpha(th.TextColor, 120)
								dims := editorWidget.Layout(gtx)

								if gtx.Focused(editor) {
									DrawFocusRing(gtx, th.BorderColorFocused, dims.Size, gtx.Dp(unit.Dp(8)))
								}
								return dims
							})
						})
					}),
				)
			})
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (f *FormModalStyle) Show() {
	f.Open = true
}

func (f *FormModalStyle) Hide() {
	f.Open = false
}

func (f *FormModalStyle) handleTab(gtx layout.Context) {
	var tags []any
	for i := range f.Fields {
		field := &f.Fields[i]
		if field.InputField != nil {
			tags = append(tags, field.InputField.FocusTag())
		} else {
			editor := field.Editor
			if editor == nil {
				editor = field.Input
			}
			if editor != nil {
				tags = append(tags, editor)
			}
		}
	}
	if f.SaveBtn != nil {
		tags = append(tags, f.SaveBtn)
	}
	if f.CancelBtn != nil {
		tags = append(tags, f.CancelBtn)
	}

	if len(tags) == 0 {
		return
	}

	for {
		event, ok := gtx.Event(
			key.Filter{Name: key.NameTab},
		)
		if !ok {
			break
		}

		if ev, ok := event.(key.Event); ok && ev.State == key.Press {
			if ev.Name == key.NameTab {
				forward := !ev.Modifiers.Contain(key.ModShift)
				focusedIdx := -1
				for i, tag := range tags {
					if gtx.Focused(tag) {
						focusedIdx = i
						break
					}
				}

				if focusedIdx == -1 {
					if forward {
						gtx.Execute(key.FocusCmd{Tag: tags[0]})
					} else {
						gtx.Execute(key.FocusCmd{Tag: tags[len(tags)-1]})
					}
				} else {
					if forward {
						nextIdx := (focusedIdx + 1) % len(tags)
						gtx.Execute(key.FocusCmd{Tag: tags[nextIdx]})
					} else {
						prevIdx := (focusedIdx - 1 + len(tags)) % len(tags)
						gtx.Execute(key.FocusCmd{Tag: tags[prevIdx]})
					}
				}
			}
		}
	}
}

// HandleShortcuts processes keyboard shortcuts for the modal
// Returns true if a shortcut was handled
// This should be called by views to let modals handle their own shortcuts before view shortcuts
func (f *FormModal) HandleShortcuts(gtx layout.Context) bool {
	if !f.Visible {
		return false
	}

	for {
		ev, ok := gtx.Event(
			key.Filter{Name: key.NameEscape},
		)
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			if ke.Name == key.NameEscape {
				f.Hide()
				if f.OnCancel != nil {
					f.OnCancel()
				}
				return true
			}
		}
	}
	return false
}
