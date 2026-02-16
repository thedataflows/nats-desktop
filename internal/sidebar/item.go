package sidebar

import (
	"image"
	"image/color"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"

	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

type Item struct {
	Tag      any
	Name     string
	Icon     *widget.Icon
	Disabled bool
}

type renderItem struct {
	Item
	hovering bool
	selected bool
	widget.Clickable
	*AlphaPalette

	next, prev any
}

type AlphaPalette struct {
	Hover, Selected uint8
}

func (r *renderItem) SetNavigation(next, prev any) {
	r.next = next
	r.prev = prev
}

func (r *renderItem) Clicked(gtx layout.Context) bool {
	if r.Item.Disabled {
		return false
	}

	if r.Clickable.Clicked(gtx) {
		return true
	}

	for {
		ev, ok := gtx.Event(key.Filter{
			Focus: &r.Clickable,
			Name:  key.NameReturn,
		}, key.Filter{
			Focus: &r.Clickable,
			Name:  key.NameEnter,
		})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			return true
		}
	}
	return false
}

func (r *renderItem) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	// Don't process hover events for disabled items
	if !r.Item.Disabled {
		for {
			ev, ok := gtx.Event(pointer.Filter{
				Target: r,
				Kinds:  pointer.Enter | pointer.Leave,
			})
			if !ok {
				break
			}

			if t, ok := ev.(pointer.Event); ok {
				switch t.Kind {
				case pointer.Enter:
					r.hovering = true
				case pointer.Leave:
					r.hovering = false
				case pointer.Cancel:
					r.hovering = false
				default:
					continue
				}
			}
		}
	}

	for {
		ev, ok := gtx.Event(key.Filter{
			Focus: &r.Clickable,
			Name:  key.NameTab,
		})
		if !ok {
			break
		}

		if e, ok := ev.(key.Event); ok && e.State == key.Press && e.Name == key.NameTab {
			if e.Modifiers == key.ModShift {
				if r.prev != nil {
					gtx.Execute(key.FocusCmd{Tag: r.prev})
				}
			} else {
				if r.next != nil {
					gtx.Execute(key.FocusCmd{Tag: r.next})
				}
			}
		}
	}

	defer pointer.PassOp{}.Push(gtx.Ops).Pop()
	defer clip.Rect(image.Rectangle{
		Max: gtx.Constraints.Max,
	}).Push(gtx.Ops).Pop()

	if r.Item.Disabled {
		// For disabled items, just render without clickable wrapper
		return layout.Inset{
			Top:    unit.Dp(4),
			Bottom: unit.Dp(4),
			Left:   unit.Dp(8),
			Right:  unit.Dp(8),
		}.Layout(gtx, func(gtx C) D {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx C) D { return r.layoutBackground(gtx, th) }),
				layout.Stacked(func(gtx C) D { return r.layoutContent(gtx, th) }),
			)
		})
	}

	return layout.Inset{
		Top:    unit.Dp(4),
		Bottom: unit.Dp(4),
		Left:   unit.Dp(8),
		Right:  unit.Dp(8),
	}.Layout(gtx, func(gtx C) D {
		return material.Clickable(gtx, &r.Clickable, func(gtx C) D {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx C) D { return r.layoutBackground(gtx, th) }),
				layout.Stacked(func(gtx C) D { return r.layoutContent(gtx, th) }),
			)
		})
	})
}

func (r *renderItem) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	gtx.Constraints.Min = gtx.Constraints.Max
	contentColor := mulAlpha(th.SideBarTextColor, 200)
	if r.selected {
		contentColor = th.SideBarTextColor
	}
	if r.Item.Disabled {
		contentColor = mulAlpha(th.SideBarTextColor, 80)
	}

	return layout.Inset{
		Left:  unit.Dp(2),
		Right: unit.Dp(2),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle, Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				if r.Item.Icon == nil {
					return layout.Dimensions{}
				}
				return layout.Inset{Bottom: unit.Dp(5), Top: unit.Dp(5)}.Layout(gtx,
					func(gtx C) D {
						iconSize := gtx.Dp(unit.Dp(24))
						gtx.Constraints = layout.Exact(image.Pt(iconSize, iconSize))
						return r.Item.Icon.Layout(gtx, contentColor)
					})
			}),
			layout.Rigid(func(gtx C) D {
				label := material.Label(th.Material(), unit.Sp(12), r.Name)
				label.Color = contentColor
				return layout.Center.Layout(gtx, label.Layout)
			}),
		)
	})
}

func mulAlpha(c color.NRGBA, alpha uint8) color.NRGBA {
	return color.NRGBA{
		R: c.R,
		G: c.G,
		B: c.B,
		A: uint8(uint32(c.A) * uint32(alpha) / 255),
	}
}

func (r *renderItem) layoutBackground(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if r.Item.Disabled {
		return layout.Dimensions{}
	}

	if !r.selected && !r.hovering {
		return layout.Dimensions{}
	}

	var fill color.NRGBA
	if r.hovering {
		fill = WithAlpha(th.Palette.ContrastBg, r.AlphaPalette.Hover)
	} else if r.selected {
		fill = th.ActionButtonBgColor
	}

	rr := gtx.Dp(unit.Dp(8))
	defer clip.RRect{
		Rect: image.Rectangle{
			Max: gtx.Constraints.Max,
		},
		NE: rr,
		SE: rr,
		NW: rr,
		SW: rr,
	}.Push(gtx.Ops).Pop()
	paintRect(gtx, gtx.Constraints.Max, fill)

	if gtx.Focused(&r.Clickable) {
		paint.FillShape(gtx.Ops, th.ContrastBg, clip.Stroke{
			Path:  clip.RRect{Rect: image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y), NE: rr, SE: rr, NW: rr, SW: rr}.Path(gtx.Ops),
			Width: float32(gtx.Dp(unit.Dp(2))),
		}.Op())
	}

	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func WithAlpha(c color.NRGBA, a uint8) color.NRGBA {
	return color.NRGBA{
		R: c.R,
		G: c.G,
		B: c.B,
		A: a,
	}
}

func paintRect(gtx layout.Context, size image.Point, fill color.NRGBA) {
	Rect{
		Color: fill,
		Size:  size,
	}.Layout(gtx)
}

type Rect struct {
	Color color.NRGBA
	Size  image.Point
	Radii int
}

func (r Rect) Layout(gtx C) D {
	paint.FillShape(
		gtx.Ops,
		r.Color,
		clip.UniformRRect(
			image.Rectangle{
				Max: r.Size,
			},
			r.Radii,
		).Op(gtx.Ops))
	return layout.Dimensions{Size: r.Size}
}
