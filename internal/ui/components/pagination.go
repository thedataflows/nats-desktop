package components

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type Pagination struct {
	CurrentPage int
	TotalPages  int
	PrevClick   *widget.Clickable
	NextClick   *widget.Clickable
}

func NewPagination(totalPages int) *Pagination {
	if totalPages < 1 {
		totalPages = 1
	}
	return &Pagination{
		CurrentPage: 1,
		TotalPages:  totalPages,
		PrevClick:   &widget.Clickable{},
		NextClick:   &widget.Clickable{},
	}
}

func (p *Pagination) SetTotalPages(total int) {
	if total < 1 {
		total = 1
	}
	p.TotalPages = total
	if p.CurrentPage > p.TotalPages {
		p.CurrentPage = p.TotalPages
	}
}

func (p *Pagination) NextPage() bool {
	if p.CurrentPage < p.TotalPages {
		p.CurrentPage++
		return true
	}
	return false
}

func (p *Pagination) PrevPage() bool {
	if p.CurrentPage > 1 {
		p.CurrentPage--
		return true
	}
	return false
}

func (p *Pagination) FocusTag() any {
	return p.PrevClick
}

func (p *Pagination) Next(gtx layout.Context) bool {
	if p.NextClick == nil {
		p.NextClick = &widget.Clickable{}
	}
	return p.NextClick.Clicked(gtx)
}

func (p *Pagination) Prev(gtx layout.Context) bool {
	if p.PrevClick == nil {
		p.PrevClick = &widget.Clickable{}
	}
	return p.PrevClick.Clicked(gtx)
}

func (p *Pagination) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if p.PrevClick == nil {
		p.PrevClick = &widget.Clickable{}
	}
	if p.NextClick == nil {
		p.NextClick = &widget.Clickable{}
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.CurrentPage <= 1 {
				gtx = gtx.Disabled()
			}
			btn := material.Button(th.Material(), p.PrevClick, "Previous")
			btn.Background = th.Palette.ContrastBg
			btn.Color = th.Palette.ContrastFg
			btn.Inset = layout.UniformInset(unit.Dp(8))
			dims := btn.Layout(gtx)
			if gtx.Focused(p.PrevClick) {
				DrawFocusRing(gtx, th.BorderColorFocused, dims.Size, gtx.Dp(unit.Dp(4)))
			}
			return dims
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			txt := fmt.Sprintf("Page %d of %d", p.CurrentPage, p.TotalPages)
			lbl := material.Label(th.Material(), unit.Sp(14), txt)
			lbl.Color = th.TextColor
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.CurrentPage >= p.TotalPages {
				gtx = gtx.Disabled()
			}
			btn := material.Button(th.Material(), p.NextClick, "Next")
			btn.Background = th.Palette.ContrastBg
			btn.Color = th.Palette.ContrastFg
			btn.Inset = layout.UniformInset(unit.Dp(8))
			dims := btn.Layout(gtx)
			if gtx.Focused(p.NextClick) {
				DrawFocusRing(gtx, th.BorderColorFocused, dims.Size, gtx.Dp(unit.Dp(4)))
			}
			return dims
		}),
	)
}
