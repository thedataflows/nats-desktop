package components

import (
	"image"
	"image/color"

	"gioui.org/io/input"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

const (
	FlatButtonIconStart = 0
	FlatButtonIconEnd   = 1
	FlatButtonIconTop   = 2
	FlatButtonIconDown  = 3
)

type FlatButton struct {
	Icon         *widget.Icon
	IconPosition int
	SpaceBetween unit.Dp

	Clickable *widget.Clickable

	MinWidth        unit.Dp
	BackgroundColor color.NRGBA
	TextColor       color.NRGBA
	Text            string

	CornerRadius      int
	BackgroundPadding unit.Dp
	ContentPadding    unit.Dp
}

func (f *FlatButton) SetIcon(icon *widget.Icon, position int, spaceBetween unit.Dp) {
	f.Icon = icon
	f.IconPosition = position
	f.SpaceBetween = spaceBetween
}

func (f *FlatButton) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if f.TextColor == (color.NRGBA{}) {
		f.TextColor = th.Palette.ContrastFg
	}

	axis := layout.Horizontal
	labelLayout := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		l := material.Label(th.Material(), unit.Sp(12), f.Text)
		l.Color = f.TextColor
		if f.Clickable.Hovered() || gtx.Focused(f.Clickable) {
			l.Color = Invert(f.TextColor)
		}
		return l.Layout(gtx)
	})

	widgets := []layout.FlexChild{labelLayout}

	if f.Icon != nil {
		iconLayout := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(f.SpaceBetween).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				icoColor := f.TextColor
				if f.Clickable.Hovered() || gtx.Focused(f.Clickable) {
					icoColor = Invert(f.TextColor)
				}
				return f.Icon.Layout(gtx, icoColor)
			})
		})

		if f.IconPosition == FlatButtonIconTop || f.IconPosition == FlatButtonIconDown {
			axis = layout.Vertical
		}

		switch f.IconPosition {
		case FlatButtonIconStart, FlatButtonIconTop:
			widgets = []layout.FlexChild{iconLayout, labelLayout}
		case FlatButtonIconEnd, FlatButtonIconDown:
			widgets = []layout.FlexChild{labelLayout, iconLayout}
		}
	}

	dims := f.Clickable.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.UniformInset(f.BackgroundPadding).Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
			semantic.Button.Add(ccgtx.Ops)
			return layout.Background{}.Layout(ccgtx,
				func(cccgtx layout.Context) layout.Dimensions {
					cccgtx.Constraints.Min.X = cccgtx.Dp(f.MinWidth)
					defer clip.UniformRRect(image.Rectangle{Max: cccgtx.Constraints.Min}, f.CornerRadius).Push(cccgtx.Ops).Pop()
					background := f.BackgroundColor
					if cccgtx.Source == (input.Source{}) {
						background = Disabled(f.BackgroundColor)
					} else if f.Clickable.Hovered() {
						background = Hovered(f.BackgroundColor)
					}
					paint.Fill(cccgtx.Ops, background)
					return layout.Dimensions{Size: cccgtx.Constraints.Min}
				},
				func(cccgtx layout.Context) layout.Dimensions {
					return layout.UniformInset(f.ContentPadding).Layout(cccgtx, func(c4gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: axis, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(c4gtx, widgets...)
					})
				},
			)
		})
	})

	if gtx.Focused(f.Clickable) {
		DrawFocusRing(gtx, th.BorderColorFocused, dims.Size, gtx.Dp(unit.Dp(8)))
	}

	return dims
}
