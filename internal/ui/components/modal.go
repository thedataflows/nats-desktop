package components

import (
	"image/color"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type Modal struct {
	Backdrop    *widget.Clickable
	contentArea widget.Clickable
	// MaxWidth sets the maximum width of the modal (default: 600)
	MaxWidth unit.Dp
}

func (m *Modal) Layout(gtx layout.Context, th *theme.Theme, title string, content layout.Widget) layout.Dimensions {
	if m.Backdrop == nil {
		m.Backdrop = &widget.Clickable{}
	}

	maxWidth := m.MaxWidth
	if maxWidth == 0 {
		maxWidth = unit.Dp(600)
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			bg := color.NRGBA{A: 150}
			paint.FillShape(cgtx.Ops, bg, clip.Rect{Max: cgtx.Constraints.Max}.Op())
			return m.Backdrop.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: ccgtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			cgtx.Constraints.Max.X = cgtx.Dp(maxWidth)
			cgtx.Constraints.Min.X = cgtx.Constraints.Max.X

			// Wrap content in clickable to prevent clicks from propagating to backdrop
			// when clicking on the modal content area
			return m.contentArea.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return Card{
					Title: title,
					Inset: layout.UniformInset(unit.Dp(24)),
				}.Layout(ccgtx, th, content)
			})
		}),
	)
}

type ModalStyle struct {
	Title        string
	Message      string
	ConfirmBtn   *widget.Clickable
	CancelBtn    *widget.Clickable
	Icon         *widget.Icon
	TitleColor   *color.NRGBA
	MessageColor *color.NRGBA
	Inset        layout.Inset
	MaxWidth     unit.Dp
	ShowIcon     bool
	Backdrop     *widget.Clickable
}

func NewModalStyle(th *theme.Theme, title, message string) ModalStyle {
	titleColor := th.TextColor
	messageColor := th.TextColor
	return ModalStyle{
		Title:        title,
		Message:      message,
		TitleColor:   &titleColor,
		MessageColor: &messageColor,
		Inset:        layout.UniformInset(unit.Dp(24)),
		MaxWidth:     unit.Dp(500),
		ShowIcon:     false,
		Backdrop:     &widget.Clickable{},
	}
}

func (m ModalStyle) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	messageColor := th.TextColor
	if m.MessageColor != nil {
		messageColor = *m.MessageColor
	}

	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			if ke.Modifiers.Contain(key.ModShift) {
				if gtx.Focused(m.CancelBtn) {
					gtx.Execute(key.FocusCmd{Tag: m.ConfirmBtn})
				} else {
					gtx.Execute(key.FocusCmd{Tag: m.CancelBtn})
				}
			} else {
				if gtx.Focused(m.ConfirmBtn) {
					gtx.Execute(key.FocusCmd{Tag: m.CancelBtn})
				} else {
					gtx.Execute(key.FocusCmd{Tag: m.ConfirmBtn})
				}
			}
		}
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			bg := color.NRGBA{A: 150}
			paint.FillShape(cgtx.Ops, bg, clip.Rect{Max: cgtx.Constraints.Max}.Op())
			return m.Backdrop.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: ccgtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			cgtx.Constraints.Max.X = cgtx.Dp(m.MaxWidth)
			cgtx.Constraints.Min.X = cgtx.Constraints.Max.X

			return Card{
				Title: m.Title,
				Inset: m.Inset,
			}.Layout(cgtx, th, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						msg := material.Label(th.Material(), unit.Sp(14), m.Message)
						msg.Color = messageColor
						return msg.Layout(cccgtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
							layout.Rigid(func(ccccgtx layout.Context) layout.Dimensions {
								if m.CancelBtn != nil {
									return SecondaryButton(th, m.CancelBtn, nil, 0, "Cancel").Layout(ccccgtx, th)
								}
								return layout.Dimensions{}
							}),
							layout.Rigid(func(ccccgtx layout.Context) layout.Dimensions {
								if m.ConfirmBtn != nil {
									return Button(th, m.ConfirmBtn, nil, 0, "Confirm").Layout(ccccgtx, th)
								}
								return layout.Dimensions{}
							}),
						)
					}),
				)
			})
		}),
	)
}

type DialogStyle struct {
	Modal ModalStyle
	Open  bool
}

func Dialog(th *theme.Theme, title, message string) DialogStyle {
	return DialogStyle{
		Modal: NewModalStyle(th, title, message),
		Open:  false,
	}
}

func (d *DialogStyle) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if !d.Open {
		return layout.Dimensions{}
	}
	return d.Modal.Layout(gtx, th)
}

func (d *DialogStyle) Show() {
	d.Open = true
}

func (d *DialogStyle) Hide() {
	d.Open = false
}
