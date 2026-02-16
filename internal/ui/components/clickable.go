package components

import (
	"image"
	"image/color"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

// Clickable lays out a rectangular clickable widget without further
// decoration.
func Clickable(gtx layout.Context, button *widget.Clickable, cornerRadius unit.Dp, w layout.Widget) layout.Dimensions {
	return button.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		semantic.Button.Add(cgtx.Ops)
		return layout.Background{}.Layout(cgtx,
			func(ccgtx layout.Context) layout.Dimensions {
				rr := ccgtx.Dp(cornerRadius)
				defer clip.UniformRRect(image.Rectangle{Max: ccgtx.Constraints.Min}, rr).Push(ccgtx.Ops).Pop()
				if button.Hovered() {
					paint.Fill(ccgtx.Ops, Hovered(color.NRGBA{}))
				}
				return layout.Dimensions{Size: ccgtx.Constraints.Min}
			},
			w,
		)
	})
}
