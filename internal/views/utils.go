package views

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/thedataflows/nats-desktop/internal/config"
	"github.com/thedataflows/nats-desktop/internal/nats"
	"github.com/thedataflows/nats-desktop/internal/navigator"
	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type App interface {
	ShowToast(message string, toastType components.ToastType)
	SetStatus(status string, connected bool)
	SetContextName(name string)
	UpdateStatusText(text string)
	UpdateAutoRefresh(enabled bool, interval string)
	GetNatsClient() *nats.Client
	NATS() *nats.Client
	GetBenchmarkManager() any // Returns *application.BenchmarkManager, but we use any to avoid circular dependency
	GetPreferences() *config.Preferences
	GetConfig() *config.Config
	SaveConfig() error
	ToggleTheme()
	Invalidate()
	GetCurrentPageID() navigator.PageId
	ShowModal(w func(gtx layout.Context) layout.Dimensions)
	HideModal()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func HandleTab(gtx layout.Context, shift bool, tags []any, next, prev any) {
	curIdx := -1
	for i, tag := range tags {
		if gtx.Source.Focused(tag) {
			curIdx = i
			break
		}
	}

	if curIdx == -1 {
		if len(tags) > 0 {
			gtx.Execute(key.FocusCmd{Tag: tags[0]})
		}
	} else {
		if shift {
			if curIdx == 0 && prev != nil {
				gtx.Execute(key.FocusCmd{Tag: prev})
				return
			}
			nextIdx := (curIdx - 1 + len(tags)) % len(tags)
			gtx.Execute(key.FocusCmd{Tag: tags[nextIdx]})
		} else {
			if curIdx == len(tags)-1 && next != nil {
				gtx.Execute(key.FocusCmd{Tag: next})
				return
			}
			nextIdx := (curIdx + 1) % len(tags)
			gtx.Execute(key.FocusCmd{Tag: tags[nextIdx]})
		}
	}
}

func layoutDetailRow(gtx layout.Context, th *theme.Theme, label, value string) layout.Dimensions {
	return layoutDetailRowColored(gtx, th, label, value, th.TextColor)
}

func layoutDetailRowColored(gtx layout.Context, th *theme.Theme, label, value string, clr color.NRGBA) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				ccgtx.Constraints.Min.X = ccgtx.Dp(unit.Dp(120))
				lbl := material.Label(th.Material(), unit.Sp(14), label)
				lbl.Color = th.TextColor
				lbl.Font.Weight = font.Bold
				return lbl.Layout(ccgtx)
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				val := material.Label(th.Material(), unit.Sp(14), value)
				val.Color = clr
				return val.Layout(ccgtx)
			}),
		)
	})
}
