package components

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type StatusPillType int

const (
	StatusPillNeutral StatusPillType = iota
	StatusPillSuccess
	StatusPillError
	StatusPillWarning
)

type StatusPill struct {
	Text string
	Type StatusPillType
	Icon *widget.Icon
}

func (p StatusPill) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	var bgColor color.NRGBA
	var textColor color.NRGBA

	switch p.Type {
	case StatusPillSuccess:
		bgColor = color.NRGBA{R: 0x2e, G: 0x7d, B: 0x32, A: 0x33}   // Green 800 with 20% alpha
		textColor = color.NRGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff} // Green 500
	case StatusPillError:
		bgColor = color.NRGBA{R: 0xc6, G: 0x28, B: 0x28, A: 0x33}   // Red 800 with 20% alpha
		textColor = color.NRGBA{R: 0xef, G: 0x53, B: 0x50, A: 0xff} // Red 400
	case StatusPillWarning:
		bgColor = color.NRGBA{R: 0xef, G: 0x6c, B: 0x00, A: 0x33}   // Orange 800 with 20% alpha
		textColor = color.NRGBA{R: 0xff, G: 0x98, B: 0x00, A: 0xff} // Orange 500
	default: // Neutral
		bgColor = color.NRGBA{R: 0x75, G: 0x75, B: 0x75, A: 0x33}   // Grey 600 with 20% alpha
		textColor = color.NRGBA{R: 0x9e, G: 0x9e, B: 0x9e, A: 0xff} // Grey 500
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			defer clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(4))).Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, bgColor)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(2),
				Bottom: unit.Dp(2),
				Left:   unit.Dp(8),
				Right:  unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if p.Icon == nil {
							return layout.Dimensions{}
						}
						return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return p.Icon.Layout(gtx, textColor)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th.Material(), unit.Sp(12), p.Text)
						lbl.Color = textColor
						lbl.Font.Weight = 600
						return lbl.Layout(gtx)
					}),
				)
			})
		}),
	)
}
