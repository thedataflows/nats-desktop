package components

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type Card struct {
	Title    string
	Subtitle string
	Actions  []layout.Widget
	Inset    layout.Inset
	Flexible bool
}

func (c Card) Layout(gtx layout.Context, th *theme.Theme, w layout.Widget) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			color := th.TableBorderColor
			paint.FillShape(gtx.Ops, th.Palette.Bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
			return widget.Border{
				Color:        color,
				CornerRadius: unit.Dp(4),
				Width:        unit.Dp(1),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Min}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			ins := c.Inset
			if ins == (layout.Inset{}) {
				ins = layout.UniformInset(unit.Dp(16))
			}
			return ins.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if c.Title == "" {
							return layout.Dimensions{}
						}
						lbl := material.Label(th.Material(), unit.Sp(16), c.Title)
						lbl.Color = th.TextColor
						lbl.Font.Weight = 600
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if c.Subtitle == "" {
							return layout.Dimensions{}
						}
						lbl := material.Label(th.Material(), unit.Sp(14), c.Subtitle)
						lbl.Color = th.SecondaryTextColor
						return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, lbl.Layout)
					}),
					func() layout.FlexChild {
						if c.Flexible {
							return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								if w == nil {
									return layout.Dimensions{}
								}
								return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, w)
							})
						} else {
							return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if w == nil {
									return layout.Dimensions{}
								}
								return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, w)
							})
						}
					}(),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if len(c.Actions) == 0 {
							return layout.Dimensions{}
						}
						return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							var flexItems []layout.FlexChild
							for _, action := range c.Actions {
								flexItems = append(flexItems, layout.Rigid(action))
								flexItems = append(flexItems, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
							}
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, flexItems[:len(flexItems)-1]...)
						})
					}),
				)
			})
		}),
	)
}

type StatCard struct {
	Title string
	Value string
}

func (s StatCard) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return Card{
		Inset: layout.UniformInset(unit.Dp(12)),
	}.Layout(gtx, th, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(12), s.Title)
				lbl.Color = th.SecondaryTextColor
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				val := material.Label(th.Material(), unit.Sp(18), s.Value)
				val.Color = th.TextColor
				val.Font.Weight = 700
				return val.Layout(gtx)
			}),
		)
	})
}
