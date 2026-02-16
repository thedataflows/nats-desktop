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
	"gioui.org/x/component"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type IconButton struct {
	Icon                 *widget.Icon
	Size                 unit.Dp
	Color                color.NRGBA
	BackgroundColor      color.NRGBA
	BackgroundColorHover color.NRGBA
	Tooltip              string
	TipArea              *component.TipArea

	SkipFocus bool
	Clickable *widget.Clickable

	OnClick func()
}

func (ib *IconButton) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if ib.BackgroundColorHover == (color.NRGBA{}) {
		ib.BackgroundColorHover = Hovered(ib.BackgroundColor)
	}

	for ib.Clickable.Clicked(gtx) {
		if ib.OnClick != nil {
			ib.OnClick()
		}
	}

	content := func(lgtx layout.Context) layout.Dimensions {
		dims := ib.Clickable.Layout(lgtx, func(cgtx layout.Context) layout.Dimensions {
			semantic.Button.Add(cgtx.Ops)
			if ib.Tooltip != "" {
				semantic.DescriptionOp(ib.Tooltip).Add(cgtx.Ops)
			}

			return layout.Background{}.Layout(cgtx,
				func(ccgtx layout.Context) layout.Dimensions {
					defer clip.UniformRRect(image.Rectangle{Max: ccgtx.Constraints.Min}, 4).Push(ccgtx.Ops).Pop()
					background := ib.BackgroundColor
					if ccgtx.Source == (input.Source{}) {
						background = Disabled(ib.BackgroundColor)
					} else if ib.Clickable.Hovered() || (ccgtx.Focused(ib.Clickable) && !ib.SkipFocus) {
						background = ib.BackgroundColorHover
					}
					paint.Fill(ccgtx.Ops, background)
					return layout.Dimensions{Size: ccgtx.Constraints.Min}
				},
				func(ccgtx layout.Context) layout.Dimensions {
					size := ccgtx.Dp(ib.Size)
					ccgtx.Constraints.Min = image.Pt(size, size)
					ccgtx.Constraints.Max = image.Pt(size, size)
					icoColor := ib.Color
					if ib.Clickable.Hovered() || (ccgtx.Focused(ib.Clickable) && !ib.SkipFocus) {
						icoColor = Invert(ib.Color)
					}
					return ib.Icon.Layout(ccgtx, icoColor)
				},
			)
		})

		if lgtx.Focused(ib.Clickable) && !ib.SkipFocus {
			DrawFocusRing(lgtx, th.BorderColorFocused, dims.Size, lgtx.Dp(unit.Dp(8)))
		}

		return dims
	}

	if ib.Tooltip != "" && ib.TipArea != nil {
		tip := component.DesktopTooltip(th.Material(), ib.Tooltip)
		return layout.Stack{}.Layout(gtx,
			layout.Stacked(content),
			layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
				return ib.TipArea.Layout(cgtx, tip, func(lgtx layout.Context) layout.Dimensions {
					return layout.Dimensions{
						Size: image.Pt(cgtx.Constraints.Min.X, cgtx.Constraints.Min.Y+cgtx.Dp(unit.Dp(12))),
					}
				})
			}),
		)
	}

	return content(gtx)
}
