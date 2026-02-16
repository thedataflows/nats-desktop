package components

import (
	"image"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type StatusBarStyle struct {
	Text            string
	ContextName     string
	Status          string
	Connected       bool
	AutoRefresh     bool
	RefreshInterval string
	Version         string
}

func StatusBar(th *theme.Theme) StatusBarStyle {
	return StatusBarStyle{
		Text:            "Ready",
		ContextName:     "",
		Status:          "Disconnected",
		Connected:       false,
		AutoRefresh:     false,
		RefreshInterval: "false",
	}
}

func (s *StatusBarStyle) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bgColor := th.TableBorderColor
			paint.FillShape(gtx.Ops, bgColor, clip.Rect{
				Max: gtx.Constraints.Max,
			}.Op())

			borderColor := th.TableBorderColor
			borderWidth := gtx.Dp(unit.Dp(1))
			paint.FillShape(gtx.Ops, borderColor, clip.Rect{
				Max: image.Pt(gtx.Constraints.Max.X, borderWidth),
			}.Op())

			return layout.Dimensions{}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutStatusIndicator(gtx, th)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								label := material.Label(th.Material(), unit.Sp(12), s.Text)
								label.Color = th.TextColor
								label.Font.Weight = font.Medium
								return label.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									size := image.Pt(gtx.Dp(unit.Dp(1)), gtx.Dp(unit.Dp(12)))
									paint.FillShape(gtx.Ops, th.TableBorderColor, clip.Rect{Max: size}.Op())
									return layout.Dimensions{Size: size}
								})
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								val := "false"
								if s.AutoRefresh {
									val = s.RefreshInterval
								}
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										label := material.Label(th.Material(), unit.Sp(12), "Auto Refresh: ")
										label.Color = th.TextColor
										label.Font.Weight = font.Medium
										return label.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										label := material.Label(th.Material(), unit.Sp(12), val)
										label.Color = th.TextColor
										label.Font.Weight = font.Bold
										return label.Layout(gtx)
									}),
								)
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							statusText := s.Status
							if s.Connected && s.ContextName != "" {
								statusText = "Connected: " + s.ContextName
							}
							label := material.Label(th.Material(), unit.Sp(12), statusText)
							label.Color = th.TextColor
							label.Font.Weight = font.Medium
							return label.Layout(gtx)
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						versionText := s.Version
						if versionText == "" {
							versionText = "dev"
						}
						label := material.Label(th.Material(), unit.Sp(11), "Version: "+versionText)
						label.Color = th.TextColor
						label.Font.Weight = font.Light
						return label.Layout(gtx)
					}),
				)
			})
		}),
	)
}

func (s *StatusBarStyle) layoutStatusIndicator(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	size := gtx.Dp(unit.Dp(10))
	gtx.Constraints = layout.Exact(image.Pt(size, size))

	bgColor := theme.LightRed
	if s.Connected {
		bgColor = theme.LightGreen
	}

	defer clip.UniformRRect(
		image.Rectangle{
			Max: image.Pt(size, size),
		},
		gtx.Dp(unit.Dp(8)),
	).Push(gtx.Ops).Pop()

	paint.FillShape(gtx.Ops, bgColor, clip.Rect{
		Max: image.Pt(size, size),
	}.Op())

	return layout.Dimensions{Size: image.Pt(size, size)}
}
