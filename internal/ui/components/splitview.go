package components

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/x/component"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type SplitView struct {
	// Ratio keeps the current layout.
	// 0 is center, -1 completely to the left, 1 completely to the right.
	// Bar is the width for resizing the layout
	BarWidth unit.Dp
	component.Resize
}

const defaultBarWidth = unit.Dp(2)

func (s *SplitView) Layout(gtx layout.Context, th *theme.Theme, left, right layout.Widget) layout.Dimensions {
	bar := gtx.Dp(s.BarWidth)
	if bar <= 1 {
		bar = gtx.Dp(defaultBarWidth)
	}

	return s.Resize.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			if s.Resize.Axis == layout.Vertical {
				return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, left)
			}
			return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, left)
		},
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, th.Palette.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
			if s.Resize.Axis == layout.Vertical {
				return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, right)
			}
			return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, right)
		},
		func(gtx layout.Context) layout.Dimensions {
			rect := image.Rectangle{
				Max: image.Point{
					X: bar,
					Y: gtx.Constraints.Max.Y,
				},
			}

			if s.Resize.Axis == layout.Vertical {
				rect.Max.X = gtx.Constraints.Max.X
				rect.Max.Y = bar
			}

			paint.FillShape(gtx.Ops, th.SeparatorColor, clip.Rect(rect).Op())
			return layout.Dimensions{Size: rect.Max}
		},
	)
}
