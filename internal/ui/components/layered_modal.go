package components

import (
	"time"

	"gioui.org/layout"
	"gioui.org/widget/material"
	"gioui.org/x/component"
)

type LayeredModal struct {
	component.VisibilityAnimation
	component.Scrim
	Widget func(gtx layout.Context, th *material.Theme, anim *component.VisibilityAnimation) layout.Dimensions
}

func NewLayeredModal() *LayeredModal {
	m := LayeredModal{}
	m.VisibilityAnimation.State = component.Invisible
	m.VisibilityAnimation.Duration = 250 * time.Millisecond
	m.Scrim.FinalAlpha = 20
	return &m
}

func (m *LayeredModal) Layout(gtx layout.Context, th *material.Theme) layout.Dimensions {
	if !m.Visible() {
		return layout.Dimensions{}
	}
	scrimDims := m.Scrim.Layout(gtx, th, &m.VisibilityAnimation)
	if m.Widget != nil {
		_ = m.Widget(gtx, th, &m.VisibilityAnimation)
	}
	return scrimDims
}

func (m *LayeredModal) Visible() bool {
	return m.VisibilityAnimation.Visible()
}
