package components

import (
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/assets"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type HeaderStyle struct {
	Title string

	BackBtn *widget.Clickable
	MenuBtn *widget.Clickable
}

func Header(th *theme.Theme) HeaderStyle {
	return HeaderStyle{
		Title: "NATS Desktop",
	}
}

func (h *HeaderStyle) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(16),
		Left:   unit.Dp(16),
		Right:  unit.Dp(16),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				img := assets.Icon128
				height := gtx.Sp(unit.Sp(24))
				scale := float32(height) / float32(img.Size().Y)
				return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return widget.Image{
						Src:   img,
						Scale: scale,
					}.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				title := material.Label(th.Material(), unit.Sp(24), h.Title)
				title.Color = th.TextColor
				title.Font.Weight = font.Bold
				return title.Layout(gtx)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{}
			}),
		)
	})
}
