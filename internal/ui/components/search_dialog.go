package components

import (
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type SearchResult struct {
	Type string
	Name string
	View string
}

type SearchDialog struct {
	InputField *InputField
	Visible    bool
	OnClose    func()
	OnSelect   func(result SearchResult)
	Selected   int
	Results    []SearchResult
	OnUpdate   func(query string)
}

func NewSearchDialog(inputField *InputField) *SearchDialog {
	if inputField == nil {
		inputField = NewInputField("Search...")
	}
	return &SearchDialog{
		InputField: inputField,
		Results:    []SearchResult{},
	}
}

func (s *SearchDialog) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if !s.Visible {
		return layout.Dimensions{}
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bg := color.NRGBA{A: 150}
			paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

			// Click to close
			clickable := &widget.Clickable{}
			return clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if clickable.Clicked(gtx) {
					if s.OnClose != nil {
						s.OnClose()
					}
				}
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(600))
				gtx.Constraints.Min.X = gtx.Constraints.Max.X

				return Card{
					Inset: layout.UniformInset(unit.Dp(16)),
				}.Layout(gtx, th, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							s.InputField.SingleLine = true
							dims := s.InputField.Layout(gtx, th)
							if s.OnUpdate != nil {
								s.OnUpdate(s.InputField.GetText())
							}
							return dims
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if len(s.Results) == 0 {
								lbl := material.Label(th.Material(), unit.Sp(14), "No results")
								lbl.Color = th.SecondaryTextColor
								return lbl.Layout(gtx)
							}
							return s.layoutResults(gtx, th)
						}),
					)
				})
			})
		}),
	)
}

func (s *SearchDialog) layoutResults(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	children := make([]layout.FlexChild, len(s.Results))
	for i := range s.Results {
		res := s.Results[i]
		children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			clickable := &widget.Clickable{}
			for i := 0; clickable.Clicked(gtx); i++ {
				if s.OnSelect != nil {
					s.OnSelect(res)
				}
				gtx.Execute(op.InvalidateCmd{})
			}

			return clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(th.Material(), unit.Sp(12), strings.ToUpper(res.Type))
							lbl.Color = th.SecondaryTextColor
							lbl.Font.Weight = font.Bold
							gtx.Constraints.Min.X = gtx.Dp(unit.Dp(80))
							return lbl.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(th.Material(), unit.Sp(14), res.Name)
							lbl.Color = th.TextColor
							return lbl.Layout(gtx)
						}),
					)
				})
			})
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}
