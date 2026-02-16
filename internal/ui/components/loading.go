package components

import (
	"image"
	"image/color"
	"math"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type LoadingOverlay struct {
	Loading bool
	Message string
}

func (l *LoadingOverlay) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if !l.Loading {
		return layout.Dimensions{}
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bg := color.NRGBA{A: 150}
			paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return Card{
				Inset: layout.UniformInset(unit.Dp(24)),
			}.Layout(gtx, th, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return Spinner(gtx, th)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if l.Message == "" {
							return layout.Dimensions{}
						}
						lbl := material.Body1(th.Material(), l.Message)
						lbl.Color = th.TextColor
						return lbl.Layout(gtx)
					}),
				)
			})
		}),
	)
}

func Spinner(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	size := gtx.Dp(unit.Dp(32))

	t := float64(gtx.Now.UnixNano()) / float64(1e9)
	angle := math.Mod(t, 1.0) * 2 * math.Pi

	// Use dot based spinner
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		dotAngle := math.Mod(angle+a, 2*math.Pi)
		alpha := uint8(255 * (1.0 - math.Mod(dotAngle/(2*math.Pi), 1.0)))

		c := th.Palette.Fg
		c.A = alpha

		r := float64(size) * 0.4
		x := float64(size)/2 + math.Cos(a)*r
		y := float64(size)/2 + math.Sin(a)*r

		dotSize := float32(gtx.Dp(unit.Dp(4)))

		stack := op.Offset(image.Pt(int(x-float64(dotSize)/2), int(y-float64(dotSize)/2))).Push(gtx.Ops)
		cl := clip.Ellipse{Max: image.Pt(int(dotSize), int(dotSize))}.Op(gtx.Ops)
		paint.FillShape(gtx.Ops, c, cl)
		stack.Pop()
	}

	gtx.Execute(op.InvalidateCmd{})
	return layout.Dimensions{Size: image.Pt(size, size)}
}
