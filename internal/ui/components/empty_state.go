package components

import (
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type EmptyState struct {
	Title    string
	Message  string
	Icon     *widget.Icon
	Button   *widget.Clickable
	BtnText  string
	OnAction func()
}

func (e EmptyState) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if e.Icon != nil {
					return layout.Inset{Bottom: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return e.Icon.Layout(gtx, th.SecondaryTextColor)
					})
				}
				return layout.Dimensions{}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(20), e.Title)
				lbl.Color = th.TextColor
				lbl.Font.Weight = 700
				return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, lbl.Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(16), e.Message)
				lbl.Color = th.SecondaryTextColor
				lbl.Alignment = text.Middle
				return layout.Inset{Bottom: unit.Dp(24)}.Layout(gtx, lbl.Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if e.Button != nil && e.BtnText != "" {
					btn := Button(th, e.Button, nil, 0, e.BtnText)
					for e.Button.Clicked(gtx) {
						if e.OnAction != nil {
							e.OnAction()
						}
					}
					return btn.Layout(gtx, th)
				}
				return layout.Dimensions{}
			}),
		)
	})
}
