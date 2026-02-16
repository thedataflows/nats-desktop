package views

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	log "github.com/thedataflows/go-lib-log"

	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/nats"
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
)

type ServerInfo struct {
	Name    string
	ID      string
	Version string
	Host    string
	Port    int
	Clients int
	UpTime  string
	RTT     string
	State   string
}

// WatchMetrics holds real-time server metrics for the watch feature
type WatchMetrics struct {
	Connections   int
	Memory        uint64
	CPU           float64
	SlowConsumers int
	Subscriptions int
	LastUpdate    time.Time
}

// formatDuration converts a duration to human-readable format starting from nanoseconds
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "N/A"
	}
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fµs", float64(d.Nanoseconds())/1000.0)
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Nanoseconds())/1e6)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

type ClusterView struct {
	*BaseView

	servers  []*ServerInfo
	filtered []*ServerInfo

	// Extra buttons not in BaseView
	pingBtn    widget.Clickable
	reportsBtn widget.Clickable

	// Filter chips
	activeFilter  *components.FilterChip
	offlineFilter *components.FilterChip

	// Report modal
	reportModal    *components.FormModal
	reportCloseBtn widget.Clickable
	reportType     string
	reportData     interface{}
	reportLoading  bool

	// Report types
	reportTypesDropDown *components.DropDown

	// Server Watch feature
	watchBtn             widget.Clickable
	watchModal           *components.FormModal
	watching             bool
	watchCancel          context.CancelFunc
	watchMetrics         WatchMetrics
	watchRefreshInterval time.Duration
	watchSelectedServer  *ServerInfo

	next, prev any
}

func NewClusterView(th *theme.Theme) *ClusterView {
	v := &ClusterView{
		BaseView: NewBaseView(
			[]string{"Name", "ID", "Version", "Host", "Port", "Clients", "RTT", "State"},
			15,
		),
		activeFilter:  components.NewFilterChip("Active"),
		offlineFilter: components.NewFilterChip("Offline"),
	}
	v.activeFilter.SetSelected(true)
	v.SearchEditor.Placeholder = "Search servers..."
	v.SearchEditor.SetIcon(icons.ActionSearch, components.IconPositionStart)

	// Set up report modal
	v.reportModal = components.NewFormModal("Server Report")
	v.reportModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.reportModal.HideSaveButton = true

	// Set up report types dropdown
	v.reportTypesDropDown = components.NewDropDown(
		components.NewDropDownOption("Accounts").WithValue("accounts"),
		components.NewDropDownOption("Connections").WithValue("connections"),
		components.NewDropDownOption("JetStream").WithValue("jetstream"),
		components.NewDropDownOption("Memory").WithValue("memory"),
		components.NewDropDownOption("CPU").WithValue("cpu"),
	)

	// Initialize watch refresh interval
	v.watchRefreshInterval = 2 * time.Second

	// Initialize watch modal
	v.watchModal = components.NewFormModal("Watch Server")
	v.watchModal.OnClose = func() {
		v.stopWatching()
		v.RestoreListFocus = true
	}
	v.watchModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.layoutWatchContent(gtx, th)
	}
	v.watchModal.HideSaveButton = true

	return v
}

func (v *ClusterView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.ClusterPageId,
		Title: "Cluster",
		Icon:  icons.ActionSettingsEthernet,
	}
}

func (v *ClusterView) Refresh() {
	if v.App == nil || v.Loading {
		return
	}

	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		v.servers = []*ServerInfo{}
		v.EmptyState = true
		v.filterServers()
		return
	}

	// Get connected server info immediately (synchronous)
	connectedAddr := client.ConnectedAddr()
	connectedID := client.ConnectedServerID()
	connectedName := client.ConnectedServerName()
	connectedVersion := client.ConnectedServerVersion()

	// Get RTT for the connected server
	rtt, _ := client.RTT()

	// Show connected server immediately (real data from connection)
	serverMap := make(map[string]*ServerInfo)
	if connectedAddr != "" {
		name := connectedName
		if name == "" {
			name = "Current Server"
		}
		serverMap[connectedID] = &ServerInfo{
			Name:    name,
			ID:      connectedID,
			Version: connectedVersion,
			Host:    connectedAddr,
			State:   "Active",
			Port:    4222,
			RTT:     formatDuration(rtt),
			Clients: 1, // At least this connection
		}
	}

	// Get discovered servers from the connection
	discoveredServers := client.DiscoveredServers()
	for _, srvAddr := range discoveredServers {
		// Parse server address to extract host and port
		host := srvAddr
		port := 4222
		if idx := strings.LastIndex(srvAddr, ":"); idx != -1 {
			host = srvAddr[:idx]
			fmt.Sscanf(srvAddr[idx+1:], "%d", &port)
		}

		serverID := srvAddr
		serverMap[serverID] = &ServerInfo{
			Name:    srvAddr,
			ID:      serverID,
			Version: "Unknown",
			Host:    host,
			Port:    port,
			State:   "Active",
			RTT:     "N/A",
			Clients: 0,
		}
	}

	// Convert map to slice and update UI immediately with connected server
	newServers := make([]*ServerInfo, 0, len(serverMap))
	for _, srv := range serverMap {
		newServers = append(newServers, srv)
	}

	v.servers = newServers
	v.EmptyState = len(newServers) == 0
	v.filterServers()
	if v.App != nil && v.App.GetCurrentPageID() == navigator.ClusterPageId {
		v.App.Invalidate()
	}
}

func (v *ClusterView) OnEnter() {
	v.Refresh()
}

func (v *ClusterView) FirstFocusTag() any {
	return v.SearchEditor.FocusTag()
}

func (v *ClusterView) LastFocusTag() any {
	return v.Paginator.NextClick
}

func (v *ClusterView) filterServers() {
	query := strings.ToLower(v.SearchEditor.GetText())
	v.filtered = make([]*ServerInfo, 0)

	for _, server := range v.servers {
		// Check search query
		if query != "" &&
			!strings.Contains(strings.ToLower(server.Name), query) &&
			!strings.Contains(strings.ToLower(server.ID), query) &&
			!strings.Contains(strings.ToLower(server.Host), query) {
			continue
		}

		// Check state filters - include if no filters selected OR if state matches a selected filter
		if !v.activeFilter.Selected && !v.offlineFilter.Selected {
			// No filters selected, show all
			v.filtered = append(v.filtered, server)
		} else if v.activeFilter.Selected && server.State == "Active" {
			v.filtered = append(v.filtered, server)
		} else if v.offlineFilter.Selected && server.State == "Offline" {
			v.filtered = append(v.filtered, server)
		}
		// If none of the above, server is filtered out
	}

	totalPages := (len(v.filtered) + v.PerPage - 1) / v.PerPage
	if totalPages < 1 {
		totalPages = 1
	}
	v.Paginator.TotalPages = totalPages
	if v.Paginator.CurrentPage > totalPages {
		v.Paginator.CurrentPage = totalPages
	}
	v.Table.ResetWidths()

	// Trigger UI refresh after filtering
	if v.App != nil {
		v.App.Invalidate()
	}
}

func (v *ClusterView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	for v.RefreshBtn.Clicked(gtx) {
		v.Refresh()
	}

	for v.pingBtn.Clicked(gtx) {
		v.pingServers()
		log.Logger().Info().
			Str("action", "ping_servers").
			Msg("Server ping initiated")
	}

	for v.reportsBtn.Clicked(gtx) {
		v.showReportModal()
		log.Logger().Info().
			Str("action", "view_reports").
			Msg("Reports view opened")
	}

	// Handle watch button click
	for v.watchBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.watchSelectedServer = v.filtered[v.SelectedIdx]
			v.startWatching()
			log.Logger().Info().
				Str("server", v.filtered[v.SelectedIdx].Name).
				Str("action", "watch_server").
				Msg("Server watch started")
		}
	}

	// Handle report modal close
	if v.reportModal.Visible {
		for v.reportCloseBtn.Clicked(gtx) {
			v.reportModal.Visible = false
			v.RestoreListFocus = true
		}
	}

	if v.SearchEditor.Changed() {
		v.LastFilterTime = gtx.Now
	}

	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
			v.filterServers()
			v.LastFilterTime = time.Time{}
		} else {
			gtx.Execute(op.InvalidateCmd{At: v.LastFilterTime.Add(300 * time.Millisecond)})
		}
	}

	if v.activeFilter.Click != nil {
		for v.activeFilter.Click.Clicked(gtx) {
			if v.activeFilter.Selected {
				// Deselect if already selected
				v.activeFilter.SetSelected(false)
			} else {
				// Select active, deselect offline
				v.activeFilter.SetSelected(true)
				v.offlineFilter.SetSelected(false)
			}
			v.filterServers()
		}
	}

	if v.offlineFilter.Click != nil {
		for v.offlineFilter.Click.Clicked(gtx) {
			if v.offlineFilter.Selected {
				// Deselect if already selected
				v.offlineFilter.SetSelected(false)
			} else {
				// Select offline, deselect active
				v.offlineFilter.SetSelected(true)
				v.activeFilter.SetSelected(false)
			}
			v.filterServers()
		}
	}

	if v.Paginator.Next(gtx) {
		v.Paginator.NextPage()
	}

	if v.Paginator.Prev(gtx) {
		v.Paginator.PrevPage()
	}

	if v.Table.Clicked() || v.Table.SelectionChanged() {
		v.SelectedIdx = (v.Paginator.CurrentPage-1)*v.PerPage + v.Table.SelectedRow
	}

	// Only handle TAB when no modal is open
	if !v.isModalVisible() {
		for {
			ev, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				v.handleTab(gtx, ke.Modifiers.Contain(key.ModShift))
			}
		}
	}

	paint.FillShape(gtx.Ops, th.Palette.Bg, clip.Rect{
		Max: gtx.Constraints.Max,
	}.Op())

	// Main layout with Stack to overlay modals
	return layout.Stack{}.Layout(gtx,
		// Main content layer
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(32)).Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutHeader(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutActions(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutStats(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
					layout.Flexed(1, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutContent(cccgtx, th)
					}),
				)
			})
		}),
		// Report modal layer
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutReportModal(cgtx, th)
		}),
		// Watch modal layer
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutWatchModal(cgtx, th)
		}),
	)
}

func (v *ClusterView) SetApp(app App) {
	v.App = app
}

func (v *ClusterView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *ClusterView) OnBack()  {}
func (v *ClusterView) OnClose() {}
func (v *ClusterView) Close()   {}

func (v *ClusterView) pingServers() {
	if v.App == nil {
		return
	}

	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		if v.App != nil {
			v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
		}
		return
	}

	v.Loading = true
	go func() {
		defer func() { v.Loading = false }()

		// For the connected server, we can use the built-in RTT() method
		// For discovered servers, we can't directly ping them without connecting
		// So we'll update RTT for the currently connected server only
		connectedID := client.ConnectedServerID()

		// Measure RTT multiple times and take average for better accuracy
		var totalRTT time.Duration
		measurements := 0
		for i := 0; i < 3; i++ {
			if rtt, err := client.RTT(); err == nil {
				totalRTT += rtt
				measurements++
			}
			time.Sleep(50 * time.Millisecond)
		}

		if measurements > 0 {
			avgRTT := totalRTT / time.Duration(measurements)
			// Update the connected server's RTT
			for _, server := range v.servers {
				if server.ID == connectedID {
					server.RTT = formatDuration(avgRTT)
					break
				}
			}
		}

		v.filterServers()
		if v.App != nil {
			v.App.ShowToast("Server ping complete", components.ToastTypeSuccess)
			v.App.Invalidate()

			// Log ping results with context
			if measurements > 0 {
				avgRTT := totalRTT / time.Duration(measurements)
				logger := log.Logger().Info().
					Str("context", "server_ping").
					Str("server_id", connectedID).
					Dur("rtt", avgRTT).
					Int("measurements", measurements)

				// Find and add server name
				for _, server := range v.servers {
					if server.ID == connectedID {
						logger = logger.Str("server_name", server.Name)
						break
					}
				}
				logger.Msg("Ping completed")
			}
		}
	}()
}

func (v *ClusterView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			title := material.Label(th.Material(), unit.Sp(24), "Cluster Management")
			title.Font.Weight = font.Bold
			title.Color = th.TextColor
			return title.Layout(cgtx)
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

func (v *ClusterView) layoutActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	isSelected := v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered)

	refreshBtn := components.Button(th, &v.RefreshBtn, icons.NavigationRefresh, components.IconPositionStart, "Refresh")
	pingBtn := components.SecondaryButton(th, &v.pingBtn, icons.AVPlayArrow, components.IconPositionStart, "Ping All")
	reportsBtn := components.SecondaryButton(th, &v.reportsBtn, icons.EditorInsertChart, components.IconPositionStart, "Reports")
	watchBtn := components.SecondaryButton(th, &v.watchBtn, icons.AVPlayArrow, components.IconPositionStart, "Watch")

	children := []layout.FlexChild{
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return refreshBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return pingBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return reportsBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			return watchBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.SearchEditor.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.activeFilter.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(1)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.offlineFilter.Layout(cgtx, th)
		}),
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (v *ClusterView) layoutStats(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	totalClients := 0
	for _, s := range v.servers {
		totalClients += s.Clients
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatCard{
				Title: "Total Servers",
				Value: fmt.Sprintf("%d", len(v.servers)),
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatCard{
				Title: "Active Connections",
				Value: fmt.Sprintf("%d", totalClients),
			}.Layout(cgtx, th)
		}),
	)
}

func (v *ClusterView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if len(v.filtered) == 0 {
		return components.EmptyState{
			Icon:    icons.ActionSettingsEthernet,
			Title:   "No Servers Found",
			Message: "Connect to a NATS server to view cluster information.",
		}.Layout(gtx, th)
	}

	return v.Split.Layout(gtx, th,
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutServerTable(cgtx, th)
		},
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutServerDetails(cgtx, th)
		},
	)
}

func (v *ClusterView) layoutServerTable(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.Table.Rows = BuildTableRows(v.filtered, v.Paginator.CurrentPage, v.PerPage,
		func(server *ServerInfo, idx int) components.TableRow {
			return components.TableRow{
				Values: []string{
					server.Name,
					server.ID,
					server.Version,
					server.Host,
					fmt.Sprintf("%d", server.Port),
					fmt.Sprintf("%d", server.Clients),
					server.RTT,
					server.State,
				},
			}
		}, v.SelectedIdx)

	return components.Card{
		Title: "Cluster Servers",
	}.Layout(gtx, th, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				return v.Table.Layout(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(ccgtx,
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return v.Paginator.Layout(cccgtx, th)
					}),
				)
			}),
		)
	})
}

func (v *ClusterView) layoutServerDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return components.EmptyState{
			Icon:    icons.ActionInfo,
			Title:   "No Server Selected",
			Message: "Select a server from the list to view its details.",
		}.Layout(gtx, th)
	}

	server := v.filtered[v.SelectedIdx]

	return components.Card{
		Title: "Server: " + server.Name,
	}.Layout(gtx, th, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return components.StatCard{
							Title: "Version",
							Value: server.Version,
						}.Layout(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return components.StatCard{
							Title: "Clients",
							Value: fmt.Sprintf("%d", server.Clients),
						}.Layout(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return components.StatCard{
							Title: "State",
							Value: server.State,
						}.Layout(cccgtx, th)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return layoutDetailRow(cccgtx, th, "Server ID", server.ID)
					}),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return layoutDetailRow(cccgtx, th, "Host", server.Host)
					}),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return layoutDetailRow(cccgtx, th, "Port", fmt.Sprintf("%d", server.Port))
					}),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return layoutDetailRow(cccgtx, th, "RTT", server.RTT)
					}),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						if server.UpTime != "" {
							return layoutDetailRow(cccgtx, th, "Uptime", server.UpTime)
						}
						return layout.Dimensions{}
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutServerHealthInfo(ccgtx, th, server)
			}),
		)
	})
}

// layoutServerHealthInfo renders health information for a specific server
func (v *ClusterView) layoutServerHealthInfo(gtx layout.Context, th *theme.Theme, server *ServerInfo) layout.Dimensions {
	// Generate health check results for this server
	results := v.getServerHealthChecks(server)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(16), "Health Status")
			lbl.Color = th.TextColor
			lbl.Font.Weight = font.Bold
			return lbl.Layout(ccgtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
			if len(results) == 0 {
				lbl := material.Label(th.Material(), unit.Sp(14), "✓ All systems healthy")
				lbl.Color = color.NRGBA{R: 34, G: 139, B: 34, A: 255}
				return lbl.Layout(ccgtx)
			}
			return v.layoutHealthResults(ccgtx, th, results)
		}),
	)
}

// getServerHealthChecks generates health check results for a specific server
func (v *ClusterView) getServerHealthChecks(server *ServerInfo) []HealthCheckResult {
	var results []HealthCheckResult

	// Check 1: Server state
	if server.State != "Active" {
		results = append(results, HealthCheckResult{
			Severity: "critical",
			Category: "connection",
			Message:  "Server is offline or unreachable",
			Server:   server.ID,
		})
	}

	// Check 2: High latency
	if server.State == "Active" && server.RTT != "" {
		if strings.Contains(server.RTT, "ms") {
			rttStr := strings.TrimSpace(strings.TrimSuffix(server.RTT, "ms"))
			if rttFloat, err := strconv.ParseFloat(rttStr, 64); err == nil && rttFloat > 100 {
				results = append(results, HealthCheckResult{
					Severity: "warning",
					Category: "performance",
					Message:  fmt.Sprintf("High RTT detected: %s", server.RTT),
					Server:   server.ID,
				})
			}
		}
	}

	// Check 3: High client count
	if server.Clients > 1000 {
		results = append(results, HealthCheckResult{
			Severity: "info",
			Category: "performance",
			Message:  fmt.Sprintf("High client count: %d clients", server.Clients),
			Server:   server.ID,
		})
	}

	return results
}

func (v *ClusterView) handleTab(gtx layout.Context, shift bool) {
	// If any modal is open, don't handle tab navigation here
	// The modals handle their own TAB navigation
	if v.isModalVisible() {
		return
	}

	tags := []any{
		&v.RefreshBtn,
		&v.pingBtn,
		&v.reportsBtn,
	}

	// Add watch button only when a server is selected
	if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
		tags = append(tags, &v.watchBtn)
	}

	tags = append(tags,
		v.SearchEditor.FocusTag(),
		v.activeFilter.FocusTag(),
		v.offlineFilter.FocusTag(),
	)

	if !v.EmptyState && len(v.filtered) > 0 {
		tags = append(tags,
			v.Table.FocusTag(),
			v.Paginator.PrevClick,
			v.Paginator.NextClick,
		)
	}

	HandleTab(gtx, shift, tags, v.next, v.prev)
}

// showReportModal shows the report type selection modal
func (v *ClusterView) showReportModal() {
	v.reportModal.Title = "Select Report Type"
	v.reportModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.reportTypesDropDown.Layout(gtx, th)
	}
	v.reportModal.OnSave = func() bool {
		selected := v.reportTypesDropDown.GetSelected()
		if selected != nil {
			v.generateReport(selected.Value)
			return true
		}
		return false
	}
	v.reportModal.Show()
}

// generateReport generates the selected report
func (v *ClusterView) generateReport(reportType string) {
	if v.App == nil {
		return
	}

	v.reportType = reportType
	v.reportLoading = true
	v.reportModal.Title = fmt.Sprintf("%s Report", strings.Title(reportType))
	v.reportModal.Visible = true

	go func() {
		defer func() { v.reportLoading = false }()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client := v.App.GetNatsClient()
		if client == nil || !client.IsConnected() {
			v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
			return
		}

		switch reportType {
		case "accounts":
			v.reportData = v.generateAccountsReport(ctx, client)
		case "connections":
			v.reportData = v.generateConnectionsReport(ctx, client)
		case "jetstream":
			v.reportData = v.generateJetStreamReport(ctx, client)
		case "memory":
			v.reportData = v.generateMemoryReport(ctx, client)
		case "cpu":
			v.reportData = v.generateCPUReport(ctx, client)
		}

		if v.App != nil && v.App.GetCurrentPageID() == navigator.ClusterPageId {
			v.App.Invalidate()
		}

		// Log report generation
		log.Logger().Info().
			Str("report_type", reportType).
			Int("servers", len(v.servers)).
			Str("server_id", client.ConnectedServerID()).
			Str("server_name", client.ConnectedServerName()).
			Msg("Cluster report generated")
	}()
}

// generateAccountsReport generates accounts report
func (v *ClusterView) generateAccountsReport(ctx context.Context, client *nats.Client) interface{} {
	return map[string]interface{}{
		"message": "Account report requires system account access",
		"servers": len(v.servers),
	}
}

// generateConnectionsReport generates connections report
func (v *ClusterView) generateConnectionsReport(ctx context.Context, client *nats.Client) interface{} {
	totalClients := 0
	for _, s := range v.servers {
		totalClients += s.Clients
	}
	return map[string]interface{}{
		"total_connections": totalClients,
		"servers":           len(v.servers),
	}
}

// generateJetStreamReport generates JetStream report
func (v *ClusterView) generateJetStreamReport(ctx context.Context, client *nats.Client) interface{} {
	info, err := client.GetAccountInfo(ctx)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{
		"streams":   info.Streams,
		"consumers": info.Consumers,
		"memory":    info.Memory,
		"storage":   info.Store,
	}
}

// generateMemoryReport generates memory usage report
func (v *ClusterView) generateMemoryReport(ctx context.Context, client *nats.Client) interface{} {
	return map[string]interface{}{
		"message": "Memory report requires system account access",
		"servers": len(v.servers),
	}
}

// generateCPUReport generates CPU usage report
func (v *ClusterView) generateCPUReport(ctx context.Context, client *nats.Client) interface{} {
	return map[string]interface{}{
		"message": "CPU report requires system account access",
		"servers": len(v.servers),
	}
}

// HealthCheckResult represents a single health check result
type HealthCheckResult struct {
	Severity string // "critical", "warning", "info"
	Category string // "connection", "performance", "jetstream", "cluster"
	Message  string
	Server   string // Server ID or "all"
}

// generateHealthReport generates comprehensive health report
func (v *ClusterView) generateHealthReport(ctx context.Context, client *nats.Client) interface{} {
	var results []HealthCheckResult

	// Check 1: Connection health
	healthy := 0
	unhealthy := 0
	for _, s := range v.servers {
		if s.State == "Active" {
			healthy++
		} else {
			unhealthy++
		}
	}

	if unhealthy > 0 {
		results = append(results, HealthCheckResult{
			Severity: "critical",
			Category: "connection",
			Message:  fmt.Sprintf("%d server(s) are offline or unreachable", unhealthy),
			Server:   "all",
		})
	}

	// Check 2: High latency servers
	for _, s := range v.servers {
		if s.State == "Active" && s.RTT != "" {
			// Parse RTT to detect slow servers (e.g., > 100ms)
			if strings.Contains(s.RTT, "ms") {
				rttStr := strings.TrimSpace(strings.TrimSuffix(s.RTT, "ms"))
				if rttFloat, err := strconv.ParseFloat(rttStr, 64); err == nil && rttFloat > 100 {
					results = append(results, HealthCheckResult{
						Severity: "warning",
						Category: "performance",
						Message:  fmt.Sprintf("High RTT detected: %s", s.RTT),
						Server:   s.ID,
					})
				}
			}
		}
	}

	// Check 3: JetStream availability
	if client != nil {
		js := client.GetJetStream()
		if js == nil {
			results = append(results, HealthCheckResult{
				Severity: "warning",
				Category: "jetstream",
				Message:  "JetStream is not available or not enabled",
				Server:   "all",
			})
		}
	}

	// Check 4: Single server cluster warning
	if len(v.servers) == 1 {
		results = append(results, HealthCheckResult{
			Severity: "info",
			Category: "cluster",
			Message:  "Single server deployment - no high availability",
			Server:   v.servers[0].ID,
		})
	}

	// Check 5: Server version consistency
	if len(v.servers) > 1 {
		versions := make(map[string]bool)
		for _, s := range v.servers {
			versions[s.Version] = true
		}
		if len(versions) > 1 {
			results = append(results, HealthCheckResult{
				Severity: "warning",
				Category: "cluster",
				Message:  fmt.Sprintf("Servers are running different versions: %d unique versions", len(versions)),
				Server:   "all",
			})
		}
	}

	// Check 6: High client count warning
	for _, s := range v.servers {
		if s.Clients > 1000 {
			results = append(results, HealthCheckResult{
				Severity: "info",
				Category: "performance",
				Message:  fmt.Sprintf("High client count: %d clients", s.Clients),
				Server:   s.ID,
			})
		}
	}

	return map[string]interface{}{
		"healthy":   healthy,
		"unhealthy": unhealthy,
		"total":     len(v.servers),
		"results":   results,
	}
}

// runHealthCheck runs a quick health check
func (v *ClusterView) runHealthCheck() {
	if v.App == nil {
		return
	}

	v.reportType = "health"
	v.reportLoading = true
	v.reportModal.Title = "Health Check"
	v.reportModal.Visible = true

	go func() {
		defer func() { v.reportLoading = false }()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := v.App.GetNatsClient()
		if client == nil || !client.IsConnected() {
			v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
			return
		}

		v.reportData = v.generateHealthReport(ctx, client)

		if v.App != nil && v.App.GetCurrentPageID() == navigator.ClusterPageId {
			v.App.Invalidate()
		}
	}()
}

// layoutReportModal renders the report modal
func (v *ClusterView) layoutReportModal(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if !v.reportModal.Visible {
		return layout.Dimensions{}
	}

	// Register for events - use clip area to capture all events within modal bounds
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, v)
	area.Pop()

	// Handle close button
	for v.reportCloseBtn.Clicked(gtx) {
		v.reportModal.Visible = false
		v.RestoreListFocus = true
	}

	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			v.reportModal.Visible = false
			v.RestoreListFocus = true
		}
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			paint.FillShape(cgtx.Ops, color.NRGBA{A: 150}, clip.Rect{Max: cgtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: cgtx.Constraints.Max}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			maxWidth := gtx.Dp(unit.Dp(600))
			maxHeight := gtx.Dp(unit.Dp(450))
			if cgtx.Constraints.Max.X > maxWidth {
				cgtx.Constraints.Max.X = maxWidth
			}
			if cgtx.Constraints.Max.Y > maxHeight {
				cgtx.Constraints.Max.Y = maxHeight
			}
			cgtx.Constraints.Min.X = gtx.Dp(unit.Dp(400))
			cgtx.Constraints.Min.Y = gtx.Dp(unit.Dp(200))

			return components.Card{}.Layout(cgtx, th, func(cccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
					layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(c4gtx,
							layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: unit.Dp(16)}.Layout(c5gtx, func(c6gtx layout.Context) layout.Dimensions {
									lbl := material.Label(th.Material(), unit.Sp(18), v.reportModal.Title)
									lbl.Color = th.TextColor
									return lbl.Layout(c6gtx)
								})
							}),
							layout.Rigid(func(c5gtx layout.Context) layout.Dimensions {
								return layout.Inset{Right: unit.Dp(16)}.Layout(c5gtx, func(c6gtx layout.Context) layout.Dimensions {
									btn := components.IconButton{
										Icon:      icons.NavigationClose,
										Clickable: &v.reportCloseBtn,
										Size:      unit.Dp(24),
										Color:     th.TextColor,
									}
									return btn.Layout(c6gtx, th)
								})
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Flexed(1, func(c4gtx layout.Context) layout.Dimensions {
						if v.reportLoading {
							return layout.Center.Layout(c4gtx, func(c5gtx layout.Context) layout.Dimensions {
								lbl := material.Label(th.Material(), unit.Sp(14), "Loading report...")
								lbl.Color = th.TextColor
								return lbl.Layout(c5gtx)
							})
						}

						if v.reportData == nil {
							return layout.Center.Layout(c4gtx, func(c5gtx layout.Context) layout.Dimensions {
								lbl := material.Label(th.Material(), unit.Sp(14), "No data available")
								lbl.Color = th.TextColor
								return lbl.Layout(c5gtx)
							})
						}

						return v.layoutReportData(c4gtx, th)
					}),
				)
			})
		}),
	)
}

// layoutReportData renders the report data
func (v *ClusterView) layoutReportData(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		data, ok := v.reportData.(map[string]interface{})
		if !ok {
			lbl := material.Label(th.Material(), unit.Sp(14), "Invalid report data")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		}

		// Default layout for reports
		children := make([]layout.FlexChild, 0, len(data)*2)
		i := 0
		for key, value := range data {
			if key == "results" {
				continue
			}
			k := key
			val := value
			children = append(children,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return layoutDetailRow(ccgtx, th, strings.Title(k), fmt.Sprintf("%v", val))
				}),
			)
			if i < len(data)-1 {
				children = append(children,
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				)
			}
			i++
		}

		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx, children...)
	})
}

// layoutHealthReportData renders the detailed health check report
func (v *ClusterView) layoutHealthReportData(gtx layout.Context, th *theme.Theme, summary map[string]interface{}, results []HealthCheckResult) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Summary section
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			healthy, _ := summary["healthy"].(int)
			unhealthy, _ := summary["unhealthy"].(int)
			total, _ := summary["total"].(int)

			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEvenly}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return components.StatCard{
						Title: "Healthy",
						Value: fmt.Sprintf("%d", healthy),
					}.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return components.StatCard{
						Title: "Unhealthy",
						Value: fmt.Sprintf("%d", unhealthy),
					}.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return components.StatCard{
						Title: "Total",
						Value: fmt.Sprintf("%d", total),
					}.Layout(ccgtx, th)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
		// Results section
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if len(results) == 0 {
				lbl := material.Label(th.Material(), unit.Sp(14), "All systems healthy!")
				lbl.Color = th.TextColor
				return layout.Center.Layout(cgtx, lbl.Layout)
			}
			return v.layoutHealthResults(cgtx, th, results)
		}),
	)
}

// layoutHealthResults renders the list of health check results
func (v *ClusterView) layoutHealthResults(gtx layout.Context, th *theme.Theme, results []HealthCheckResult) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(results)*2)
	for i, result := range results {
		r := result
		children = append(children,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.layoutHealthResultRow(cgtx, th, r)
			}),
		)
		if i < len(results)-1 {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			)
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// layoutHealthResultRow renders a single health check result
func (v *ClusterView) layoutHealthResultRow(gtx layout.Context, th *theme.Theme, result HealthCheckResult) layout.Dimensions {
	var statusType components.StatusPillType
	switch result.Severity {
	case "critical":
		statusType = components.StatusPillError
	case "warning":
		statusType = components.StatusPillWarning
	case "info":
		statusType = components.StatusPillNeutral
	default:
		statusType = components.StatusPillNeutral
	}

	return components.Card{}.Layout(gtx, th, func(cgtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
				layout.Rigid(func(c3gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(c3gtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return components.StatusPill{
								Text: strings.ToUpper(result.Severity),
								Type: statusType,
							}.Layout(c4gtx, th)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							lbl := material.Label(th.Material(), unit.Sp(12), strings.Title(result.Category))
							lbl.Color = th.TextColor
							lbl.Font.Weight = font.Bold
							return lbl.Layout(c4gtx)
						}),
						layout.Flexed(1, func(c4gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{}
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							if result.Server != "all" && result.Server != "" {
								lbl := material.Label(th.Material(), unit.Sp(11), result.Server)
								lbl.Color = color.NRGBA{R: 128, G: 128, B: 128, A: 255}
								return lbl.Layout(c4gtx)
							}
							return layout.Dimensions{}
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(c3gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th.Material(), unit.Sp(13), result.Message)
					lbl.Color = th.TextColor
					return lbl.Layout(c3gtx)
				}),
			)
		})
	})
}

// layoutReportRow renders a single report row
func (v *ClusterView) layoutReportRow(gtx layout.Context, th *theme.Theme, key string, value interface{}) layout.Dimensions {
	var valueStr string
	switch v := value.(type) {
	case string:
		valueStr = v
	case int:
		valueStr = fmt.Sprintf("%d", v)
	case int64:
		valueStr = fmt.Sprintf("%d", v)
	case uint64:
		valueStr = fmt.Sprintf("%d", v)
	case float64:
		valueStr = fmt.Sprintf("%.2f", v)
	default:
		valueStr = fmt.Sprintf("%v", v)
	}

	return layoutDetailRow(gtx, th, strings.Title(key), valueStr)
}

// startWatching starts polling server metrics every 2 seconds
func (v *ClusterView) startWatching() {
	if v.App == nil || v.watching {
		return
	}

	if v.watchSelectedServer == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	v.watchCancel = cancel
	v.watching = true
	v.watchModal.Show()

	// Initial metrics fetch
	v.refreshWatchMetrics()

	// Start polling
	go func() {
		ticker := time.NewTicker(v.watchRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				v.refreshWatchMetrics()
				if v.App != nil {
					v.App.Invalidate()
				}
			}
		}
	}()
}

// stopWatching stops the watch
func (v *ClusterView) stopWatching() {
	if v.watchCancel != nil {
		v.watchCancel()
		v.watchCancel = nil
	}
	v.watching = false
}

// isModalVisible checks if any modal is open
func (v *ClusterView) isModalVisible() bool {
	return v.reportModal.Visible || v.watchModal.Visible
}

// refreshWatchMetrics fetches current server metrics
func (v *ClusterView) refreshWatchMetrics() {
	if v.App == nil || v.watchSelectedServer == nil {
		return
	}

	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		return
	}

	// Update metrics from connection stats
	conn := client.Conn()
	if conn != nil {
		stats := conn.Stats()
		if conn.ConnectedUrl() != "" {
			v.watchMetrics.Connections = 1
		} else {
			v.watchMetrics.Connections = 0
		}
		v.watchMetrics.Memory = stats.InMsgs // Using InMsgs as a proxy for memory
		v.watchMetrics.LastUpdate = time.Now()
	}

	// Update with server info from the selected server
	v.watchMetrics.Connections = v.watchSelectedServer.Clients
	v.watchMetrics.Subscriptions = 0 // Would need actual subscription count
	v.watchMetrics.CPU = 0           // Would need system account access
	v.watchMetrics.SlowConsumers = 0 // Would need system account access
}

// layoutWatchModal renders the watch modal
func (v *ClusterView) layoutWatchModal(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.watchSelectedServer != nil {
		v.watchModal.Title = "Watch: " + v.watchSelectedServer.Name
	}
	return v.watchModal.Layout(gtx, th)
}

// layoutWatchContent renders the watch modal content with metrics
func (v *ClusterView) layoutWatchContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			// Live indicator with pulsing dot
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(ccgtx,
					layout.Rigid(func(c3gtx layout.Context) layout.Dimensions {
						// Pulsing dot indicator
						dotSize := c3gtx.Dp(unit.Dp(8))
						var dotColor color.NRGBA

						// Pulse based on current time
						elapsed := time.Since(v.watchMetrics.LastUpdate).Milliseconds()
						if elapsed%2000 < 1000 {
							dotColor = color.NRGBA{R: 76, G: 175, B: 80, A: 255} // Green
						} else {
							dotColor = color.NRGBA{R: 76, G: 175, B: 80, A: 100} // Faded green
						}

						// Draw circle using Ellipse
						paint.FillShape(c3gtx.Ops, dotColor, clip.Ellipse{
							Max: image.Pt(dotSize, dotSize),
						}.Op(c3gtx.Ops))
						return layout.Dimensions{Size: image.Pt(dotSize, dotSize)}
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(c3gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th.Material(), unit.Sp(14), "LIVE")
						lbl.Color = color.NRGBA{R: 76, G: 175, B: 80, A: 255}
						lbl.Font.Weight = font.Bold
						return lbl.Layout(c3gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					layout.Rigid(func(c3gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th.Material(), unit.Sp(12), v.watchMetrics.LastUpdate.Format("15:04:05"))
						lbl.Color = color.NRGBA{R: 128, G: 128, B: 128, A: 255}
						return lbl.Layout(c3gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			// Metrics grid
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutWatchMetricsGrid(ccgtx, th)
			}),
		)
	})
}

// layoutWatchMetricsGrid renders the metrics grid with bar charts
func (v *ClusterView) layoutWatchMetricsGrid(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutWatchMetricCard(cgtx, th, "Connections", fmt.Sprintf("%d", v.watchMetrics.Connections), v.watchMetrics.Connections, 100, color.NRGBA{R: 33, G: 150, B: 243, A: 255})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			memoryMB := float64(v.watchMetrics.Memory) / (1024 * 1024)
			return v.layoutWatchMetricCard(cgtx, th, "Memory", fmt.Sprintf("%.2f MB", memoryMB), int(memoryMB), 1000, color.NRGBA{R: 156, G: 39, B: 176, A: 255})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutWatchMetricCard(cgtx, th, "CPU", fmt.Sprintf("%.1f%%", v.watchMetrics.CPU), int(v.watchMetrics.CPU), 100, color.NRGBA{R: 255, G: 152, B: 0, A: 255})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutWatchMetricCard(cgtx, th, "Slow Consumers", fmt.Sprintf("%d", v.watchMetrics.SlowConsumers), v.watchMetrics.SlowConsumers, 50, color.NRGBA{R: 244, G: 67, B: 54, A: 255})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutWatchMetricCard(cgtx, th, "Subscriptions", fmt.Sprintf("%d", v.watchMetrics.Subscriptions), v.watchMetrics.Subscriptions, 1000, color.NRGBA{R: 76, G: 175, B: 80, A: 255})
		}),
	)
}

// layoutWatchMetricCard renders a single metric card with bar chart
func (v *ClusterView) layoutWatchMetricCard(gtx layout.Context, th *theme.Theme, label, value string, current, max int, barColor color.NRGBA) layout.Dimensions {
	return components.Card{}.Layout(gtx, th, func(cgtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16), Top: unit.Dp(12), Bottom: unit.Dp(12)}.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
				// Label and value row
				layout.Rigid(func(c3gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(c3gtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							lbl := material.Label(th.Material(), unit.Sp(14), label)
							lbl.Color = th.TextColor
							return lbl.Layout(c4gtx)
						}),
						layout.Flexed(1, func(c4gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{}
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							lbl := material.Label(th.Material(), unit.Sp(16), value)
							lbl.Color = th.TextColor
							lbl.Font.Weight = font.Bold
							return lbl.Layout(c4gtx)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				// Bar chart
				layout.Rigid(func(c3gtx layout.Context) layout.Dimensions {
					// Background bar
					barHeight := c3gtx.Dp(unit.Dp(8))
					maxWidth := c3gtx.Constraints.Max.X
					if maxWidth == 0 {
						maxWidth = 200
					}

					// Calculate bar width based on current/max ratio
					var barWidth int
					if max > 0 {
						ratio := float64(current) / float64(max)
						if ratio > 1 {
							ratio = 1
						}
						barWidth = int(float64(maxWidth) * ratio)
					}
					if barWidth < 2 {
						barWidth = 2
					}

					// Background (track)
					bgColor := color.NRGBA{R: 200, G: 200, B: 200, A: 100}
					paint.FillShape(c3gtx.Ops, bgColor, clip.RRect{
						Rect: image.Rectangle{Max: image.Pt(maxWidth, barHeight)},
						SE:   barHeight / 2, SW: barHeight / 2, NE: barHeight / 2, NW: barHeight / 2,
					}.Op(c3gtx.Ops))

					// Fill bar
					paint.FillShape(c3gtx.Ops, barColor, clip.RRect{
						Rect: image.Rectangle{Max: image.Pt(barWidth, barHeight)},
						SE:   barHeight / 2, SW: barHeight / 2, NE: barHeight / 2, NW: barHeight / 2,
					}.Op(c3gtx.Ops))

					return layout.Dimensions{Size: image.Pt(maxWidth, barHeight)}
				}),
			)
		})
	})
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *ClusterView) HandleShortcuts(gtx layout.Context) bool {
	// Check if any modal is visible first - view shortcuts disabled when modal is open
	if v.isModalVisible() {
		return false
	}

	// View-specific shortcuts - process only ONE event per frame
	ev, ok := gtx.Event(
		key.Filter{Name: key.Name("R"), Optional: key.ModShortcut},
	)
	if !ok {
		return false
	}

	if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
		switch {
		case ke.Name == key.Name("R") && ke.Modifiers.Contain(key.ModShortcut):
			v.RefreshBtn.Click()
			return true
		}
	}
	return false
}

// GetShortcutsHelp returns help text for this view's shortcuts
func (v *ClusterView) GetShortcutsHelp() []shortcuts.Shortcut {
	return []shortcuts.Shortcut{
		shortcuts.Refresh(func() {}),
	}
}
