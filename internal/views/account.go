package views

import (
	"context"
	"fmt"
	"image"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
)

type AccountView struct {
	app        App
	info       *jetstream.AccountInfo
	loading    bool
	refreshBtn widget.Clickable

	next, prev any
}

func NewAccountView(th *theme.Theme) *AccountView {
	return &AccountView{}
}

func (v *AccountView) SetApp(app App) {
	v.app = app
}

func (v *AccountView) OnEnter() {
	v.Refresh()
}

func (v *AccountView) FirstFocusTag() any { return &v.refreshBtn }
func (v *AccountView) LastFocusTag() any  { return &v.refreshBtn }

func (v *AccountView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *AccountView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.AccountPageId,
		Title: "Account",
		Icon:  icons.ActionAccountCircle,
	}
}

func (v *AccountView) Refresh() {
	if v.app == nil || v.loading {
		return
	}

	client := v.app.GetNatsClient()
	if client == nil || !client.IsConnected() {
		v.info = nil
		return
	}

	v.loading = true
	go func() {
		defer func() { v.loading = false }()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		info, err := client.GetAccountInfo(ctx)
		if err != nil {
			v.app.ShowToast("Failed to fetch account info: "+err.Error(), components.ToastTypeError)
			return
		}

		v.info = info
		if v.app != nil && v.app.GetCurrentPageID() == navigator.AccountPageId {
			v.app.Invalidate()
		}
	}()
}

func (v *AccountView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	for v.refreshBtn.Clicked(gtx) {
		v.Refresh()
	}

	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			v.handleTab(gtx, ke.Modifiers.Contain(key.ModShift))
		}
	}

	paint.FillShape(gtx.Ops, th.Palette.Bg, clip.Rect{
		Max: gtx.Constraints.Max,
	}.Op())

	return layout.UniformInset(unit.Dp(32)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutHeader(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutContent(ccgtx, th)
			}),
		)
	})
}

func (v *AccountView) handleTab(gtx layout.Context, shift bool) {
	tags := []any{
		&v.refreshBtn,
	}

	HandleTab(gtx, shift, tags, v.next, v.prev)
}

func (v *AccountView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(cgtx,
				layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
					header := material.Label(th.Material(), unit.Sp(24), "Account Information")
					header.Color = th.TextColor
					return header.Layout(ccgtx)
				}),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					btn := components.Button(th, &v.refreshBtn, icons.NavigationRefresh, components.IconPositionStart, "Refresh")
					return btn.Layout(ccgtx, th)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			borderColor := th.TableBorderColor
			paint.FillShape(cgtx.Ops, borderColor, clip.Rect{
				Max: image.Pt(cgtx.Constraints.Max.X, cgtx.Dp(unit.Dp(1))),
			}.Op())
			return layout.Dimensions{}
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
	)
}

func (v *AccountView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutAccountDetails(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutConnectionStats(cgtx, th)
		}),
	)
}

func (v *AccountView) layoutAccountDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(18), "Account Details")
			header.Color = th.TextColor
			header.Font.Weight = 700
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(cgtx, header.Layout)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if v.info == nil {
				return components.Card{
					Title:    "No Account Info",
					Subtitle: "Connect to NATS to see account details",
				}.Layout(cgtx, th, nil)
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return components.Card{
						Title:    "JetStream Status",
						Subtitle: fmt.Sprintf("Memory: %d / %d, Storage: %d / %d", v.info.Tier.Memory, v.info.Tier.Limits.MaxMemory, v.info.Tier.Store, v.info.Tier.Limits.MaxStore),
					}.Layout(ccgtx, th, nil)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return components.Card{
						Title:    "Limits",
						Subtitle: fmt.Sprintf("Streams: %d / %d, Consumers: %d / %d", v.info.Tier.Streams, v.info.Tier.Limits.MaxStreams, v.info.Tier.Consumers, v.info.Tier.Limits.MaxConsumers),
					}.Layout(ccgtx, th, nil)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					domain := v.info.Domain
					if domain == "" {
						domain = "default"
					}
					return components.Card{
						Title:    "Domain",
						Subtitle: domain,
					}.Layout(ccgtx, th, nil)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return components.Card{
						Title:    "API Statistics",
						Subtitle: fmt.Sprintf("Total: %d, Errors: %d, Inflight: %d", v.info.API.Total, v.info.API.Errors, v.info.API.Inflight),
					}.Layout(ccgtx, th, nil)
				}),
			)
		}),
	)
}

func (v *AccountView) layoutConnectionStats(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(18), "Connection Statistics")
			header.Color = th.TextColor
			header.Font.Weight = 700
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(cgtx, header.Layout)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return components.Card{
								Title:    "Server Status",
								Subtitle: "Connected",
							}.Layout(cccgtx, th, nil)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return components.StatusPill{
								Text: "Connected",
								Type: components.StatusPillSuccess,
							}.Layout(cccgtx, th)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return components.StatCard{
						Title: "Active Connections",
						Value: "3",
					}.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return components.StatCard{
						Title: "Data Rate",
						Value: "1.2 MB/s",
					}.Layout(ccgtx, th)
				}),
			)
		}),
	)
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *AccountView) HandleShortcuts(gtx layout.Context) bool {
	// TODO: Implement view-specific shortcuts
	return false
}

// GetShortcutsHelp returns help text for this view's shortcuts
func (v *AccountView) GetShortcutsHelp() []shortcuts.Shortcut {
	return nil
}
