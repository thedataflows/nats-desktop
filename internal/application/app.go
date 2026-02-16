package application

import (
	"image/color"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"gioui.org/x/component"
	log "github.com/thedataflows/go-lib-log"

	"github.com/thedataflows/nats-desktop/internal/config"
	"github.com/thedataflows/nats-desktop/internal/nats"
	"github.com/thedataflows/nats-desktop/internal/navigator"
	"github.com/thedataflows/nats-desktop/internal/shortcuts"
	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
	"github.com/thedataflows/nats-desktop/internal/views"
)

type App struct {
	window       *app.Window
	Theme        *theme.Theme
	navigator    *navigator.Navigator
	toasts       *components.ToastManager
	modal        *components.DialogStyle
	modalLayer   *components.LayeredModal
	statusBar    *components.StatusBarStyle
	header       *components.HeaderStyle
	searchDialog *components.SearchDialog
	searchShown  bool
	lastTime     time.Time
	lastRefresh  time.Time
	cfg          *config.Config

	connectionsView  *views.ConnectionsView
	benchmarkManager *BenchmarkManager
}

func (a *App) GetNatsClient() *nats.Client {
	if a.connectionsView == nil {
		return nil
	}
	return a.connectionsView.GetNatsClient()
}

func (a *App) GetBenchmarkManager() any {
	client := a.GetNatsClient()
	if client == nil || !client.IsConnected() {
		return nil
	}

	if a.benchmarkManager == nil {
		a.benchmarkManager = NewBenchmarkManager(client.Conn(), client.JetStream())
	} else {
		a.benchmarkManager.nc = client.Conn()
		a.benchmarkManager.js = client.JetStream()
	}

	return a.benchmarkManager
}

func (a *App) NATS() *nats.Client {
	return a.GetNatsClient()
}

func (a *App) GetPreferences() *config.Preferences {
	return &a.cfg.Preferences
}

func (a *App) GetConfig() *config.Config {
	return a.cfg
}

func (a *App) SaveConfig() error {
	return a.cfg.Save()
}

func (a *App) ToggleTheme() {
	isDark := a.cfg.Preferences.Theme == "dark"
	newDark := !isDark
	if newDark {
		a.cfg.Preferences.Theme = "dark"
	} else {
		a.cfg.Preferences.Theme = "light"
	}
	a.Theme.Switch(newDark)
	if err := a.cfg.Save(); err == nil {
		// saved
	}
	a.Invalidate()
}

func (a *App) ShowToast(message string, toastType components.ToastType) {
	switch toastType {
	case components.ToastTypeSuccess:
		a.toasts.ShowSuccess(message, a.Theme)
	case components.ToastTypeWarning:
		a.toasts.ShowWarning(message, a.Theme)
	case components.ToastTypeError:
		a.toasts.ShowError(message, a.Theme)
	default:
		a.toasts.ShowInfo(message, a.Theme)
	}
}

func (a *App) SetStatus(status string, connected bool) {
	// Log significant status changes (connection/disconnection events)
	if connected != a.statusBar.Connected {
		if connected {
			log.Logger().Info().Str("status", status).Msg("Connected")
		} else {
			log.Logger().Info().Str("status", status).Msg("Disconnected")
		}
	}

	a.statusBar.Status = status
	a.statusBar.Connected = connected
	if !connected {
		a.statusBar.ContextName = ""
	}
	a.statusBar.Text = "Ready"
}

func (a *App) SetContextName(name string) {
	// Log context changes
	if name != a.statusBar.ContextName {
		log.Logger().Info().Str("context", name).Msg("Context changed")
	}
	a.statusBar.ContextName = name
}

func (a *App) UpdateStatusText(txt string) {
	a.statusBar.Text = txt
}

func (a *App) UpdateAutoRefresh(enabled bool, interval string) {
	a.statusBar.AutoRefresh = enabled
	a.statusBar.RefreshInterval = interval
}

func (a *App) Invalidate() {
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) GetCurrentPageID() navigator.PageId {
	if a.navigator == nil || a.navigator.Current() == nil {
		return ""
	}
	return a.navigator.Current().Info().ID
}

func (a *App) ShowModal(w func(gtx layout.Context) layout.Dimensions) {
	a.modalLayer.VisibilityAnimation = component.VisibilityAnimation{
		Duration: 200 * time.Millisecond,
		State:    component.Invisible,
	}

	a.modalLayer.Widget = func(gtx layout.Context, mth *material.Theme, anim *component.VisibilityAnimation) layout.Dimensions {
		gtx.Constraints.Min = gtx.Constraints.Max
		paint.Fill(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 128})
		return w(gtx)
	}

	a.modalLayer.VisibilityAnimation.Appear(time.Now())
}

func (a *App) HideModal() {
	a.modalLayer.VisibilityAnimation.Disappear(time.Now())
}

// HandleKeyEvent processes global keyboard events
// This is called from the main event loop for key events
func HandleKeyEvent(a *App, event key.Event) bool {
	// Global shortcuts are now handled in Layout() via gtx.Event
	// This function is kept for compatibility but shortcuts are processed during layout
	return false
}

func (a *App) ShowShortcutsHelp() {
	// Get shortcuts from current view
	if currentView := a.navigator.Current(); currentView != nil {
		viewShortcuts := currentView.GetShortcutsHelp()
		if len(viewShortcuts) > 0 {
			helpText := "Keyboard Shortcuts:\n\n"
			for _, sc := range viewShortcuts {
				helpText += shortcuts.FormatShortcut(sc) + " - " + sc.HelpText + "\n"
			}
			a.ShowToast(helpText, components.ToastTypeInfo)
		} else {
			a.ShowToast("No shortcuts available for this view", components.ToastTypeInfo)
		}
	}
}

func (a *App) ToggleSearch() {
	a.searchShown = !a.searchShown
	a.searchDialog.Visible = a.searchShown
	if a.searchShown {
		a.performSearch()
	}
}

func (a *App) setupSearchDialog() {
	a.searchDialog.OnClose = func() {
		a.searchShown = false
		a.searchDialog.Visible = false
	}
	a.searchDialog.OnSelect = func(result components.SearchResult) {
		switch result.View {
		case "connections":
			a.navigator.SwitchTo(navigator.ConnectionsPageId)
		case "streams":
			a.navigator.SwitchTo(navigator.StreamsPageId)
		case "consumers":
			a.navigator.SwitchTo(navigator.ConsumersPageId)
		case "cluster":
			a.navigator.SwitchTo(navigator.ClusterPageId)
		case "kv":
			a.navigator.SwitchTo(navigator.KVPageId)
		case "objects":
			a.navigator.SwitchTo(navigator.ObjectsPageId)
		case "services":
			a.navigator.SwitchTo(navigator.ServicesPageId)
		case "pubsub":
			a.navigator.SwitchTo(navigator.PubSubPageId)
		case "benchmarks":
			a.navigator.SwitchTo(navigator.BenchmarksPageId)
		case "events":
			a.navigator.SwitchTo(navigator.EventsPageId)
		case "backup":
			a.navigator.SwitchTo(navigator.BackupPageId)
		case "schema":
			a.navigator.SwitchTo(navigator.SchemaPageId)
		case "account":
			a.navigator.SwitchTo(navigator.AccountPageId)
		case "preferences":
			a.navigator.SwitchTo(navigator.PreferencesPageId)
		}
	}
	a.searchDialog.OnUpdate = func(query string) {
		if len(query) == 0 {
			a.searchDialog.Results = []components.SearchResult{}
			return
		}

		results := []components.SearchResult{}
		allResults := []components.SearchResult{
			{Type: "View", Name: "Connections", View: "connections"},
			{Type: "View", Name: "Streams", View: "streams"},
			{Type: "View", Name: "Consumers", View: "consumers"},
			{Type: "View", Name: "Cluster", View: "cluster"},
			{Type: "View", Name: "Key-Value", View: "kv"},
			{Type: "View", Name: "Object Stores", View: "objects"},
			{Type: "View", Name: "Services", View: "services"},
			{Type: "View", Name: "Pub/Sub", View: "pubsub"},
			{Type: "View", Name: "Benchmarks", View: "benchmarks"},
			{Type: "View", Name: "Events", View: "events"},
			{Type: "View", Name: "Backup", View: "backup"},
			{Type: "View", Name: "Schema", View: "schema"},
			{Type: "View", Name: "Account", View: "account"},
			{Type: "View", Name: "Auth", View: "auth"},
			{Type: "View", Name: "Settings", View: "settings"},
		}

		lowerQuery := strings.ToLower(query)
		for _, result := range allResults {
			if strings.Contains(strings.ToLower(result.Name), lowerQuery) {
				results = append(results, result)
			}
		}

		a.searchDialog.Results = results
	}
}

func (a *App) performSearch() {
	query := a.searchDialog.InputField.GetText()
	if len(query) == 0 {
		a.searchDialog.Results = []components.SearchResult{}
		return
	}

	results := []components.SearchResult{
		{Type: "View", Name: "Connections", View: "connections"},
		{Type: "View", Name: "Streams", View: "streams"},
		{Type: "View", Name: "Consumers", View: "consumers"},
		{Type: "View", Name: "Cluster", View: "cluster"},
		{Type: "View", Name: "Key-Value", View: "kv"},
		{Type: "View", Name: "Object Stores", View: "objects"},
		{Type: "View", Name: "Services", View: "services"},
		{Type: "View", Name: "Pub/Sub", View: "pubsub"},
		{Type: "View", Name: "Benchmarks", View: "benchmarks"},
		{Type: "View", Name: "Events", View: "events"},
		{Type: "View", Name: "Backup", View: "backup"},
		{Type: "View", Name: "Schema", View: "schema"},
		{Type: "View", Name: "Account", View: "account"},
		{Type: "View", Name: "Settings", View: "settings"},
	}

	a.searchDialog.Results = results
}

func NewApp(w *app.Window, version string) *App {
	// Log application startup with version
	log.Logger().Info().Str("version", version).Msg("Application starting")

	cfg := config.NewConfig()
	cfg.Load()

	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	appTheme := theme.New(th, cfg.Preferences.Theme == "dark")
	navi := navigator.New()

	statusBar := components.StatusBar(appTheme)
	statusBar.Status = "No connection"
	statusBar.AutoRefresh = cfg.Preferences.AutoRefresh
	statusBar.RefreshInterval = cfg.Preferences.RefreshPeriod.String()
	statusBar.Version = version

	header := components.Header(appTheme)

	appInstance := &App{
		window:       w,
		Theme:        appTheme,
		navigator:    navi,
		toasts:       components.NewToastManager(),
		modalLayer:   components.NewLayeredModal(),
		statusBar:    &statusBar,
		header:       &header,
		searchDialog: components.NewSearchDialog(components.NewInputField("")),
		searchShown:  false,
		cfg:          cfg,
	}

	// Set up the connection check function for the navigator
	appInstance.navigator.SetConnectionCheckFunc(func() bool {
		client := appInstance.GetNatsClient()
		return client != nil && client.IsConnected()
	})

	appInstance.registerViews(appInstance)
	appInstance.setupSearchDialog()

	appInstance.navigator.SwitchTo(navigator.ConnectionsPageId)

	return appInstance
}

func (a *App) registerViews(appl *App) {
	cv := views.NewConnectionsView(a.Theme)
	cv.SetApp(appl)
	a.connectionsView = cv

	clv := views.NewClusterView(a.Theme)
	clv.SetApp(appl)

	sv := views.NewStreamsView(a.Theme)
	sv.SetApp(appl)

	csv := views.NewConsumersView(a.Theme)
	csv.SetApp(appl)

	kvv := views.NewKVView(a.Theme)
	kvv.SetApp(appl)
	ov := views.NewObjectsView(a.Theme)
	ov.SetApp(appl)
	srv := views.NewServicesView(a.Theme)
	srv.SetApp(appl)
	ps := views.NewPubSubView(a.Theme)
	ps.SetApp(appl)
	bmv := views.NewBenchmarksView(a.Theme)
	bmv.SetApp(appl)
	trv := views.NewTraceView(a.Theme)
	trv.SetApp(appl)
	ev := views.NewEventsView(a.Theme)
	ev.SetApp(appl)
	bkp := views.NewBackupView(a.Theme)
	bkp.SetApp(appl)
	schv := views.NewSchemaView(a.Theme)
	schv.SetApp(appl)
	acc := views.NewAccountView(a.Theme)
	acc.SetApp(appl)
	pref := views.NewPreferencesView(a.Theme)
	pref.SetApp(appl)
	counter := views.NewCounterView(a.Theme)
	counter.SetApp(appl)
	audit := views.NewAuditView(a.Theme)
	audit.SetApp(appl)

	a.navigator.Register(cv)
	a.navigator.Register(clv)
	a.navigator.Register(sv)
	a.navigator.Register(csv)
	a.navigator.Register(kvv)
	a.navigator.Register(ov)
	a.navigator.Register(srv)
	a.navigator.Register(ps)
	a.navigator.Register(bmv)
	a.navigator.Register(trv)
	a.navigator.Register(ev)
	a.navigator.Register(bkp)
	a.navigator.Register(schv)
	a.navigator.Register(acc)
	a.navigator.Register(counter)
	a.navigator.Register(audit)
	a.navigator.Register(pref)
}

// handleGlobalShortcuts processes global navigation shortcuts (always active)
// Returns true if a shortcut was handled
func (a *App) handleGlobalShortcuts(gtx layout.Context) bool {
	// Process only ONE key event per frame to prevent key-repeat flooding
	ev, ok := gtx.Event(
		key.Filter{Name: key.Name("1"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("!"), Optional: key.ModShortcut | key.ModShift},
		key.Filter{Name: key.Name("2"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("@"), Optional: key.ModShortcut | key.ModShift},
		key.Filter{Name: key.Name("3"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("#"), Optional: key.ModShortcut | key.ModShift},
		key.Filter{Name: key.Name("4"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("$"), Optional: key.ModShortcut | key.ModShift},
		key.Filter{Name: key.Name("5"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("%"), Optional: key.ModShortcut | key.ModShift},
		key.Filter{Name: key.Name("6"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("^"), Optional: key.ModShortcut | key.ModShift},
		key.Filter{Name: key.Name("7"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("&"), Optional: key.ModShortcut | key.ModShift},
		key.Filter{Name: key.Name("8"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("9"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("0"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("H"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name(","), Optional: key.ModShortcut},
	)
	if !ok {
		return false
	}

	if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
		switch {
		case ke.Name == key.Name("1") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(1)
			return true
		case ke.Name == key.Name("2") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(2)
			return true
		case ke.Name == key.Name("3") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(3)
			return true
		case ke.Name == key.Name("4") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(4)
			return true
		case ke.Name == key.Name("5") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(5)
			return true
		case ke.Name == key.Name("6") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(6)
			return true
		case ke.Name == key.Name("7") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(7)
			return true
		case ke.Name == key.Name("8") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(8)
			return true
		case ke.Name == key.Name("9") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(9)
			return true
		case ke.Name == key.Name("0") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(10)
			return true
		case ke.Name == key.Name("!") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(11)
			return true
		case ke.Name == key.Name("@") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(12)
			return true
		case ke.Name == key.Name("#") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(13)
			return true
		case ke.Name == key.Name("$") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(14)
			return true
		case ke.Name == key.Name("%") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(15)
			return true
		case ke.Name == key.Name("^") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(16)
			return true
		case ke.Name == key.Name("&") && ke.Modifiers.Contain(key.ModCtrl):
			a.navigator.Sidebar.TriggerClickByIndex(17)
			return true
		case ke.Name == key.Name("H") && ke.Modifiers.Contain(key.ModCtrl):
			a.ShowShortcutsHelp()
			return true
		case ke.Name == key.Name(",") && ke.Modifiers.Contain(key.ModCtrl):
			// Preferences is typically the last item, try index 9
			count := a.navigator.Sidebar.GetItemCount()
			if count > 0 {
				a.navigator.Sidebar.TriggerClickByIndex(count)
			}
			return true
		}
	}
	return false
}

// globalShortcutsTag is used to identify global shortcut event handlers
var globalShortcutsTag struct{}

// Layout renders the application UI
func Layout(gtx layout.Context, a *App) layout.Dimensions {
	// Register for global key events across the entire window
	// This must be done before processing events
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &globalShortcutsTag)
	area.Pop()

	// Handle global navigation shortcuts first (always active)
	// Don't return early - just process the shortcut and continue with layout
	// to prevent white screen when holding keys
	a.handleGlobalShortcuts(gtx)

	// Then let current view handle its shortcuts
	// Don't return early - just process the shortcut and continue with layout
	if currentView := a.navigator.Current(); currentView != nil {
		currentView.HandleShortcuts(gtx)
	}

	a.navigator.Update()

	// Update focus navigation between Sidebar and Current View
	if cv := a.navigator.Current(); cv != nil {
		cv.SetNavigation(a.navigator.FirstFocusTag(), a.navigator.LastFocusTag())
		a.navigator.SetNavigationLinks(cv.FirstFocusTag(), cv.LastFocusTag())
	}

	if !a.lastTime.IsZero() {
		dt := gtx.Now.Sub(a.lastTime)
		a.toasts.Update(dt)
	}
	a.lastTime = gtx.Now

	if a.cfg.Preferences.AutoRefresh {
		if r, ok := a.navigator.Current().(navigator.Refreshable); ok {
			if a.lastRefresh.IsZero() {
				a.lastRefresh = gtx.Now
			}

			period := a.cfg.Preferences.RefreshPeriod
			if period < time.Second {
				period = time.Second
			}

			if gtx.Now.Sub(a.lastRefresh) >= period {
				a.lastRefresh = gtx.Now
				r.Refresh()
			}
			// Schedule next refresh frame ONLY if it's in the future
			nextRefresh := a.lastRefresh.Add(period)
			if nextRefresh.After(gtx.Now) {
				gtx.Execute(op.InvalidateCmd{At: nextRefresh})
			}
		}
	}

	if a.modalLayer.Animating() {
		gtx.Execute(op.InvalidateCmd{})
	}

	if a.toasts.HasActiveToasts() {
		// Schedule redraw when the first toast is due to expire
		minDuration := a.toasts.MinHideAfter()
		if minDuration > 0 {
			gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(minDuration)})
		} else {
			// If we have toasts but minDuration is 0, they should likely be removed
			// We schedule a slightly delayed frame to let Update handle it
			gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(100 * time.Millisecond)})
		}
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			paint.Fill(cgtx.Ops, a.Theme.Palette.Bg)

			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return a.header.Layout(ccgtx, a.Theme)
				}),
				layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Spacing: 0}.Layout(ccgtx,
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return a.navigator.Layout(cccgtx, a.Theme)
						}),
						layout.Flexed(1, func(cccgtx layout.Context) layout.Dimensions {
							cccgtx.Constraints.Min = cccgtx.Constraints.Max
							return a.navigator.Current().Layout(cccgtx, a.Theme)
						}),
					)
				}),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return a.statusBar.Layout(ccgtx, a.Theme)
				}),
			)
		}),
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			cgtx.Constraints.Min = cgtx.Constraints.Max
			return a.modalLayer.Layout(cgtx, a.Theme.Material())
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if a.modal != nil && a.modal.Open {
				return a.modal.Layout(cgtx, a.Theme)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return a.searchDialog.Layout(cgtx, a.Theme)
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			cgtx.Constraints.Min = cgtx.Constraints.Max
			return layout.NE.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(24), Right: unit.Dp(24)}.Layout(ccgtx, func(cccgtx layout.Context) layout.Dimensions {
					return a.toasts.Layout(cccgtx, a.Theme)
				})
			})
		}),
	)
}
