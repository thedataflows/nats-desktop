package components

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

// DrawFocusRing draws a thin solid focus ring around the given size with optional rounded corners.
func DrawFocusRing(gtx layout.Context, c color.NRGBA, size image.Point, radius int) {
	// Exactly 1px inset to ensure it's clearly inside the widget boundary
	inset := gtx.Dp(unit.Dp(1))
	if inset < 1 {
		inset = 1
	}

	// Calculate inner radius and inset in Dp for widget.Border
	pxPerDp := float32(gtx.Dp(unit.Dp(1)))
	if pxPerDp == 0 {
		pxPerDp = 1
	}

	ifrDp := unit.Dp(float32(radius-inset) / pxPerDp)
	insetDp := unit.Dp(float32(inset) / pxPerDp)

	layout.Inset{
		Top:    insetDp,
		Bottom: insetDp,
		Left:   insetDp,
		Right:  insetDp,
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{
			X: size.X - inset*2,
			Y: size.Y - inset*2,
		}
		gtx.Constraints.Max = gtx.Constraints.Min
		return widget.Border{
			Color:        c,
			CornerRadius: ifrDp,
			Width:        unit.Dp(1),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		})
	})
}

// LayoutFocusRing overlays a solid focus ring if the element is focused.
func LayoutFocusRing(gtx layout.Context, c color.NRGBA, dims layout.Dimensions, radius int) {
	trans := op.Offset(image.Point{}).Push(gtx.Ops)
	DrawFocusRing(gtx, c, dims.Size, radius)
	trans.Pop()
}
