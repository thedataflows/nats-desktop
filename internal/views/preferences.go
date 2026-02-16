package views

import (
	"image"
	"time"

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

type PreferencesView struct {
	app               App
	themeToggle       widget.Bool
	autoRefreshToggle widget.Bool
	themeToggleCheck  components.CheckBoxStyle
	autoRefreshCheck  components.CheckBoxStyle
	refreshInterval   *components.InputField
	refreshError      string
	backupLocation    *components.InputField
	debounceTimer     *time.Timer

	// Track previous values to detect changes
	prevThemeValue       bool
	prevAutoRefreshValue bool

	next, prev any
}

func NewPreferencesView(th *theme.Theme) *PreferencesView {
	v := &PreferencesView{
		themeToggle:       widget.Bool{Value: true},
		autoRefreshToggle: widget.Bool{Value: true},
		refreshInterval:   components.NewLabeledInputFieldWithPosition("Refresh interval", "e.g., 30s", components.LabelPositionTop),
		backupLocation:    components.NewLabeledInputFieldWithPosition("Backup location", "e.g., ./jetstream-backups", components.LabelPositionTop),
	}
	v.themeToggleCheck = components.CheckBox(th.Material(), &v.themeToggle, "Dark Mode")
	v.themeToggleCheck.SetTheme(th)
	v.autoRefreshCheck = components.CheckBox(th.Material(), &v.autoRefreshToggle, "Auto-Refresh")
	v.autoRefreshCheck.SetTheme(th)
	v.refreshInterval.SetText("30s")
	v.backupLocation.SetText("./jetstream-backups")

	// Set up tab navigation for checkboxes
	v.themeToggleCheck.SetOnTab(func(gtx layout.Context, shift bool) {
		v.handleTab(gtx, shift)
	})
	v.autoRefreshCheck.SetOnTab(func(gtx layout.Context, shift bool) {
		v.handleTab(gtx, shift)
	})

	return v
}

func (v *PreferencesView) SetApp(app App) {
	v.app = app
	if app != nil && app.GetPreferences() != nil {
		prefs := app.GetPreferences()
		v.themeToggle.Value = prefs.Theme == "dark"
		v.autoRefreshToggle.Value = prefs.AutoRefresh
		v.refreshInterval.SetText(prefs.RefreshPeriod.String())
		if prefs.BackupLocation != "" {
			v.backupLocation.SetText(prefs.BackupLocation)
		}
	}
}

func (v *PreferencesView) OnEnter() {
	if v.app != nil && v.app.GetPreferences() != nil {
		prefs := v.app.GetPreferences()
		v.themeToggle.Value = prefs.Theme == "dark"
		v.autoRefreshToggle.Value = prefs.AutoRefresh
		v.refreshInterval.SetText(prefs.RefreshPeriod.String())
		if prefs.BackupLocation != "" {
			v.backupLocation.SetText(prefs.BackupLocation)
		}
	}
}

func (v *PreferencesView) FirstFocusTag() any {
	return v.themeToggleCheck.FocusTag()
}

func (v *PreferencesView) LastFocusTag() any {
	return v.backupLocation.FocusTag()
}

func (v *PreferencesView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *PreferencesView) savePreferences() {
	if v.app == nil || v.app.GetPreferences() == nil {
		return
	}
	prefs := v.app.GetPreferences()
	if v.themeToggle.Value {
		prefs.Theme = "dark"
	} else {
		prefs.Theme = "light"
	}
	prefs.AutoRefresh = v.autoRefreshToggle.Value

	if d, err := time.ParseDuration(v.refreshInterval.GetText()); err == nil {
		if d < time.Second {
			d = time.Second
			v.refreshInterval.SetText("1s")
		}
		prefs.RefreshPeriod = d
		v.refreshError = ""
	} else {
		v.refreshError = "Invalid duration"
	}

	v.app.UpdateAutoRefresh(v.autoRefreshToggle.Value, v.refreshInterval.GetText())

	// Save backup location
	prefs.BackupLocation = v.backupLocation.GetText()

	v.app.SaveConfig()
}

func (v *PreferencesView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.PreferencesPageId,
		Title: "Preferences",
		Icon:  icons.ActionSettings,
	}
}

func (v *PreferencesView) handleTab(gtx layout.Context, shift bool) {
	tags := []any{
		v.themeToggleCheck.FocusTag(),
		v.autoRefreshCheck.FocusTag(),
		v.refreshInterval.FocusTag(),
		v.backupLocation.FocusTag(),
	}
	HandleTab(gtx, shift, tags, v.next, v.prev)
}

func (v *PreferencesView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			v.handleTab(gtx, ke.Modifiers.Contain(key.ModShift))
		}
	}

	if v.refreshInterval.Changed() {
		if v.debounceTimer != nil {
			v.debounceTimer.Stop()
		}
		v.debounceTimer = time.AfterFunc(300*time.Millisecond, func() {
			v.savePreferences()
			if v.app != nil && v.app.GetCurrentPageID() == navigator.PreferencesPageId {
				v.app.Invalidate()
			}
		})
	}

	if v.backupLocation.Changed() {
		if v.debounceTimer != nil {
			v.debounceTimer.Stop()
		}
		v.debounceTimer = time.AfterFunc(300*time.Millisecond, func() {
			v.savePreferences()
			if v.app != nil && v.app.GetCurrentPageID() == navigator.PreferencesPageId {
				v.app.Invalidate()
			}
		})
	}

	// Store previous values before layout
	v.prevThemeValue = v.themeToggle.Value
	v.prevAutoRefreshValue = v.autoRefreshToggle.Value

	paint.FillShape(gtx.Ops, th.Palette.Bg, clip.Rect{
		Max: gtx.Constraints.Max,
	}.Op())

	dims := layout.UniformInset(unit.Dp(32)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
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

	// Check for value changes after layout (checkbox layouts have been called)
	if v.themeToggle.Value != v.prevThemeValue {
		v.savePreferences()
		if v.app != nil {
			v.app.ToggleTheme()
		}
	}

	if v.autoRefreshToggle.Value != v.prevAutoRefreshValue {
		v.savePreferences()
	}

	return dims
}

func (v *PreferencesView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(24), "Preferences")
			header.Color = th.TextColor
			return header.Layout(cgtx)
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

func (v *PreferencesView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutSection(cgtx, th, "Appearance")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(32)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutSection(cgtx, th, "Data Management")
		}),
	)
}

func (v *PreferencesView) layoutSection(gtx layout.Context, th *theme.Theme, title string) layout.Dimensions {
	var content layout.Widget
	switch title {
	case "Appearance":
		content = v.layoutAppearanceSettings(gtx, th)
	case "Data Management":
		content = v.layoutDataManagement(gtx, th)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(18), title)
			lbl.Color = th.TextColor
			lbl.Font.Weight = 700
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(cgtx, lbl.Layout)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.Card{}.Layout(cgtx, th, content)
		}),
	)
}

func (v *PreferencesView) layoutAppearanceSettings(gtx layout.Context, th *theme.Theme) layout.Widget {
	return func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.themeToggleCheck.Layout(ccgtx)
			}),
		)
	}
}

func (v *PreferencesView) layoutDataManagement(gtx layout.Context, th *theme.Theme) layout.Widget {
	return func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.autoRefreshCheck.Layout(ccgtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						cccgtx.Constraints.Max.X = cccgtx.Dp(unit.Dp(180))
						v.refreshInterval.Label = "Refresh Interval"
						v.refreshInterval.ShowLabel = true
						v.refreshInterval.LabelWidth = unit.Dp(112)
						v.refreshInterval.MinWidth = unit.Dp(50)
						v.refreshInterval.Hint = "30s"
						v.refreshInterval.SetError(v.refreshError)
						return v.refreshInterval.Layout(cccgtx, th)
					}),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						if v.refreshError == "" {
							return layout.Dimensions{}
						}
						return layout.Inset{Left: unit.Dp(124), Top: unit.Dp(4)}.Layout(cccgtx, func(ccccgtx layout.Context) layout.Dimensions {
							lbl := material.Label(th.Material(), unit.Sp(12), v.refreshError)
							lbl.Color = th.ErrorColor
							return lbl.Layout(ccccgtx)
						})
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						v.backupLocation.Label = "Backup Location"
						v.backupLocation.ShowLabel = true
						v.backupLocation.LabelWidth = unit.Dp(112)
						v.backupLocation.MinWidth = unit.Dp(300)
						v.backupLocation.Hint = "./jetstream-backups"
						return v.backupLocation.Layout(cccgtx, th)
					}),
				)
			}),
		)
	}
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *PreferencesView) HandleShortcuts(gtx layout.Context) bool {
	// TODO: Implement view-specific shortcuts
	return false
}

// GetShortcutsHelp returns help text for this view's shortcuts
func (v *PreferencesView) GetShortcutsHelp() []shortcuts.Shortcut {
	return nil
}
