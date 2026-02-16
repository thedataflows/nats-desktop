package components

import (
	"image/color"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type FilterChip struct {
	Label    string
	Selected bool
	Click    *widget.Clickable
	Color    color.NRGBA
}

func NewFilterChip(label string) *FilterChip {
	return &FilterChip{
		Label:    label,
		Selected: false,
		Click:    &widget.Clickable{},
	}
}

func (f *FilterChip) SetSelected(selected bool) {
	f.Selected = selected
}
func (fc *FilterChip) FocusTag() any {
	return fc.Click
}
func (f *FilterChip) SetColor(color color.NRGBA) {
	f.Color = color
}

func (f *FilterChip) Clicked(gtx layout.Context) bool {
	var clicked bool
	for f.Click.Clicked(gtx) {
		f.Selected = !f.Selected
		clicked = true
	}
	if clicked {
		gtx.Execute(op.InvalidateCmd{})
	}
	return clicked
}

func (f *FilterChip) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	f.Clicked(gtx)

	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: f.Click, Name: key.NameTab, Optional: key.ModShift},
			key.Filter{Focus: f.Click, Name: key.NameEnter},
			key.Filter{Focus: f.Click, Name: key.NameReturn},
			key.Filter{Focus: f.Click, Name: key.NameSpace},
		)
		if !ok {
			break
		}

		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			f.Selected = !f.Selected
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	var bgColor color.NRGBA
	var textColor color.NRGBA
	var borderColor color.NRGBA

	if f.Selected {
		if f.Color.A > 0 {
			bgColor = f.Color
		} else {
			bgColor = th.ActionButtonBgColor
		}
		textColor = th.ButtonTextColor
		borderColor = bgColor
	} else {
		bgColor = th.Palette.ContrastBg
		textColor = th.TextColor
		borderColor = th.BorderColor
		if f.Click.Hovered() {
			borderColor = th.Palette.ContrastFg
		}
	}

	dims := f.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return widget.Border{
			Color:        borderColor,
			Width:        unit.Dp(1),
			CornerRadius: unit.Dp(0),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, bgColor, clip.Rect{
				Max: gtx.Constraints.Min,
			}.Op())

			return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.Label(th.Material(), unit.Sp(12), f.Label)
				label.Font.Weight = font.Medium
				label.Color = textColor
				return label.Layout(gtx)
			})
		})
	})

	if gtx.Focused(f.Click) {
		DrawFocusRing(gtx, th.BorderColorFocused, dims.Size, gtx.Dp(unit.Dp(4)))
	}

	return dims
}
