package components

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

// ExpandableSection is a collapsible section that can contain any content
type ExpandableSection struct {
	Title     string
	Expanded  bool
	clickable widget.Clickable
}

func NewExpandableSection(title string) *ExpandableSection {
	return &ExpandableSection{
		Title:    title,
		Expanded: false,
	}
}

func (e *ExpandableSection) Layout(gtx layout.Context, th *theme.Theme, content layout.Widget) layout.Dimensions {
	// Handle click to toggle
	for e.clickable.Clicked(gtx) {
		e.Expanded = !e.Expanded
		gtx.Execute(op.InvalidateCmd{})
	}

	// Handle keyboard navigation when focused
	if gtx.Source.Focused(&e.clickable) {
		for {
			ev, ok := gtx.Event(
				key.Filter{Focus: &e.clickable, Name: key.NameEnter},
				key.Filter{Focus: &e.clickable, Name: key.NameReturn},
				key.Filter{Focus: &e.clickable, Name: key.NameSpace},
				key.Filter{Focus: &e.clickable, Name: key.NameUpArrow},
				key.Filter{Focus: &e.clickable, Name: key.NameDownArrow},
				key.Filter{Focus: &e.clickable, Name: key.NameLeftArrow},
				key.Filter{Focus: &e.clickable, Name: key.NameRightArrow},
			)
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				switch ke.Name {
				case key.NameEnter, key.NameReturn, key.NameSpace:
					// Toggle expand/collapse
					e.Expanded = !e.Expanded
					gtx.Execute(op.InvalidateCmd{})
				case key.NameDownArrow, key.NameRightArrow:
					// Expand on DOWN/RIGHT
					if !e.Expanded {
						e.Expanded = true
						gtx.Execute(op.InvalidateCmd{})
					}
				case key.NameUpArrow, key.NameLeftArrow:
					// Collapse on UP/LEFT
					if e.Expanded {
						e.Expanded = false
						gtx.Execute(op.InvalidateCmd{})
					}
				}
			}
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			dims := e.clickable.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						icon := icons.NavigationChevronRight
						if e.Expanded {
							icon = icons.NavigationExpandMore
						}
						btn := IconButton{
							Icon:      icon,
							Size:      unit.Dp(20),
							Color:     th.SecondaryTextColor,
							Clickable: &widget.Clickable{},
						}
						return btn.Layout(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						lbl := material.Label(th.Material(), unit.Sp(14), e.Title)
						lbl.Color = th.SecondaryTextColor
						return lbl.Layout(cccgtx)
					}),
				)
			})
			// Draw focus ring when focused
			if gtx.Source.Focused(&e.clickable) {
				dims.Size.X += gtx.Dp(unit.Dp(8))
				DrawFocusRing(gtx, th.BorderColorFocused, dims.Size, gtx.Dp(unit.Dp(8)))
			}
			return dims
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !e.Expanded {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(24)}.Layout(cgtx, content)
		}),
	)
}

// FocusTag returns the clickable widget for focus management
func (e *ExpandableSection) FocusTag() any {
	return &e.clickable
}
