package components

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type ProgressBar struct {
	Progress float64 // 0.0 to 1.0
	Color    color.NRGBA
	Height   unit.Dp
}

func (p ProgressBar) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if p.Height == 0 {
		p.Height = unit.Dp(4)
	}

	c := p.Color
	if (c == color.NRGBA{}) {
		c = th.Palette.ContrastBg
	}

	height := gtx.Dp(p.Height)
	width := gtx.Constraints.Min.X

	// Background
	bgColor := th.TableBorderColor
	rect := image.Rect(0, 0, width, height)
	paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(rect, height/2).Op(gtx.Ops))

	// Progress
	progressWidth := int(float64(width) * p.Progress)
	if progressWidth > 0 {
		if progressWidth > width {
			progressWidth = width
		}
		progressRect := image.Rect(0, 0, progressWidth, height)
		paint.FillShape(gtx.Ops, c, clip.UniformRRect(progressRect, height/2).Op(gtx.Ops))
	}

	return layout.Dimensions{
		Size: image.Point{X: width, Y: height},
	}
}
