package components

import (
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	log "github.com/thedataflows/go-lib-log"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type ToastType int

const (
	ToastTypeInfo ToastType = iota
	ToastTypeSuccess
	ToastTypeWarning
	ToastTypeError
)

type ToastStyle struct {
	Title     string
	Message   string
	Type      ToastType
	Icon      *widget.Icon
	Visible   bool
	HideAfter time.Duration
}

func Toast(th *theme.Theme, toastType ToastType, title, message string) ToastStyle {
	return ToastStyle{
		Title:     title,
		Message:   message,
		Type:      toastType,
		Visible:   true,
		HideAfter: 3 * time.Second,
	}
}

func (t *ToastStyle) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if !t.Visible {
		return layout.Dimensions{}
	}

	var bgColor color.NRGBA
	var iconColor color.NRGBA

	switch t.Type {
	case ToastTypeSuccess:
		bgColor = color.NRGBA{R: 16, G: 185, B: 129, A: 20}
		iconColor = color.NRGBA{R: 16, G: 185, B: 129, A: 255}
	case ToastTypeWarning:
		bgColor = color.NRGBA{R: 245, G: 158, B: 11, A: 20}
		iconColor = color.NRGBA{R: 245, G: 158, B: 11, A: 255}
	case ToastTypeError:
		bgColor = color.NRGBA{R: 239, G: 68, B: 68, A: 20}
		iconColor = color.NRGBA{R: 239, G: 68, B: 68, A: 255}
	default:
		bgColor = color.NRGBA{R: 59, G: 130, B: 246, A: 20}
		iconColor = color.NRGBA{R: 59, G: 130, B: 246, A: 255}
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			defer clip.UniformRRect(image.Rectangle{Max: cgtx.Constraints.Min}, cgtx.Dp(unit.Dp(4))).Push(cgtx.Ops).Pop()
			paint.Fill(cgtx.Ops, bgColor)
			return layout.Dimensions{Size: cgtx.Constraints.Min}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(12)).Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						if t.Icon != nil {
							iconSize := cccgtx.Dp(unit.Dp(20))
							cccgtx.Constraints = layout.Exact(image.Pt(iconSize, iconSize))
							return t.Icon.Layout(cccgtx, iconColor)
						}
						return layout.Dimensions{}
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
							layout.Rigid(func(ccccgtx layout.Context) layout.Dimensions {
								if t.Title != "" {
									title := material.Label(th.Material(), unit.Sp(14), t.Title)
									title.Font.Weight = font.SemiBold
									title.Color = th.TextColor
									return title.Layout(ccccgtx)
								}
								return layout.Dimensions{}
							}),
							layout.Rigid(func(ccccgtx layout.Context) layout.Dimensions {
								if t.Message != "" {
									lbl := material.Label(th.Material(), unit.Sp(13), t.Message)
									lbl.Color = th.TextColor
									return lbl.Layout(ccccgtx)
								}
								return layout.Dimensions{}
							}),
						)
					}),
				)
			})
		}),
	)
}

type ToastManager struct {
	toasts []*ToastStyle
}

func NewToastManager() *ToastManager {
	return &ToastManager{
		toasts: make([]*ToastStyle, 0, 5),
	}
}

func (tm *ToastManager) Add(toast *ToastStyle) {
	if len(tm.toasts) >= 5 {
		tm.toasts = tm.toasts[1:]
	}
	tm.toasts = append(tm.toasts, toast)
}

func (tm *ToastManager) ShowInfo(message string, th *theme.Theme) {
	log.Logger().Debug().Str("toast", "info").Msg(message)
	tm.Add(&ToastStyle{
		Message:   message,
		Type:      ToastTypeInfo,
		Visible:   true,
		HideAfter: 3 * time.Second,
	})
}

func (tm *ToastManager) ShowSuccess(message string, th *theme.Theme) {
	log.Logger().Debug().Str("toast", "success").Msg(message)
	tm.Add(&ToastStyle{
		Message:   message,
		Type:      ToastTypeSuccess,
		Visible:   true,
		HideAfter: 3 * time.Second,
	})
}

func (tm *ToastManager) ShowWarning(message string, th *theme.Theme) {
	log.Logger().Warn().Str("toast", "warning").Msg(message)
	tm.Add(&ToastStyle{
		Message:   message,
		Type:      ToastTypeWarning,
		Visible:   true,
		HideAfter: 5 * time.Second,
	})
}

func (tm *ToastManager) ShowError(message string, th *theme.Theme) {
	log.Logger().Error().Str("toast", "error").Msg(message)
	tm.Add(&ToastStyle{
		Message:   message,
		Type:      ToastTypeError,
		Visible:   true,
		HideAfter: 5 * time.Second,
	})
}

func (tm *ToastManager) Update(dt time.Duration) {
	active := tm.toasts[:0]
	for _, t := range tm.toasts {
		if t.HideAfter > 0 {
			t.HideAfter -= dt
			if t.HideAfter > 0 {
				active = append(active, t)
			}
		}
	}
	tm.toasts = active
}

func (tm *ToastManager) HasActiveToasts() bool {
	return len(tm.toasts) > 0
}

func (tm *ToastManager) MinHideAfter() time.Duration {
	if len(tm.toasts) == 0 {
		return 0
	}
	minDuration := tm.toasts[0].HideAfter
	for _, t := range tm.toasts {
		if t.HideAfter < minDuration {
			minDuration = t.HideAfter
		}
	}
	return minDuration
}

func (tm *ToastManager) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if len(tm.toasts) == 0 {
		return layout.Dimensions{}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		tm.layoutToasts(gtx, th)...,
	)
}

func (tm *ToastManager) layoutToasts(gtx layout.Context, th *theme.Theme) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(tm.toasts))
	for i := len(tm.toasts) - 1; i >= 0; i-- {
		t := tm.toasts[i]
		children = append(children, layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				ccgtx.Constraints.Max.X = ccgtx.Dp(unit.Dp(300))
				return t.Layout(ccgtx, th)
			})
		}))
	}
	return children
}
