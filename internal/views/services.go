package views

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"

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
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
)

type ServicesView struct {
	*BaseView

	services []*ServiceInfo
	filtered []*ServiceInfo

	// Extra buttons not in BaseView
	resetBtn widget.Clickable

	// Filter chips
	activeFilter *components.FilterChip
	idleFilter   *components.FilterChip

	// Stats and Ping buttons
	statsBtn widget.Clickable
	pingBtn  widget.Clickable

	// Modals
	statsModal *components.FormModal
	pingModal  *components.FormModal

	// Selected service for stats modal
	selectedService *ServiceInfo

	// Ping results
	pingResults []PingResult
	pinging     bool

	next, prev any
}

type PingResult struct {
	ServiceName string
	Status      string // "OK" or "Error"
	Latency     time.Duration
	Error       string
}

type ServiceInfo struct {
	Name       string
	Subject    string
	Type       string
	Group      string
	Endpoint   string
	Instances  int
	Calls      int64
	AvgLatency string
	Throughput string
	Status     string
	Created    string
}

func (v *ServicesView) isModalVisible() bool {
	return (v.statsModal != nil && v.statsModal.Visible) || (v.pingModal != nil && v.pingModal.Visible)
}

func (v *ServicesView) showServiceStats(service *ServiceInfo) {
	v.selectedService = service
	v.statsModal.Show()
}

func (v *ServicesView) pingAllServices() {
	if v.App == nil {
		return
	}

	v.pinging = true
	v.pingResults = []PingResult{}
	v.pingModal.Show()

	go func() {
		defer func() { v.pinging = false }()

		client := v.App.GetNatsClient()
		if client == nil || !client.IsConnected() {
			v.pingResults = append(v.pingResults, PingResult{
				ServiceName: "System",
				Status:      "Error",
				Error:       "Not connected to NATS",
			})
			if v.App != nil {
				v.App.Invalidate()
			}
			return
		}

		// Ping each service
		for _, service := range v.services {
			start := time.Now()

			// Use NATS request/reply to ping service
			// Services respond to $SRV.PING.<service_name> subject
			subject := fmt.Sprintf("$SRV.PING.%s", service.Name)
			_, err := client.Request(subject, nil, 2*time.Second)
			latency := time.Since(start)

			result := PingResult{
				ServiceName: service.Name,
				Latency:     latency,
			}

			if err != nil {
				result.Status = "Error"
				result.Error = err.Error()
			} else {
				result.Status = "OK"
			}

			v.pingResults = append(v.pingResults, result)
			if v.App != nil {
				v.App.Invalidate()
			}

			// Log each service ping result with context
			logger := log.Logger().Info().
				Str("service", service.Name).
				Str("subject", service.Subject).
				Str("endpoint", service.Endpoint).
				Str("group", service.Group).
				Int("instances", service.Instances).
				Str("status", result.Status).
				Dur("latency", latency)
			if result.Error != "" {
				logger = logger.Str("error", result.Error)
			}
			logger.Msg("Service ping completed")
		}

		// Log summary of service ping
		log.Logger().Info().
			Int("total_services", len(v.services)).
			Int("results", len(v.pingResults)).
			Msg("Service ping complete")
	}()
}

func NewServicesView(th *theme.Theme) *ServicesView {
	v := &ServicesView{
		BaseView: NewBaseView(
			[]string{"Name", "Subject", "Instances", "Calls", "Avg Latency", "Throughput", "Status"},
			15,
		),
		services:     []*ServiceInfo{},
		filtered:     []*ServiceInfo{},
		activeFilter: components.NewFilterChip("Active"),
		idleFilter:   components.NewFilterChip("Idle"),
	}
	v.activeFilter.SetSelected(true)
	v.SearchEditor.Placeholder = "Search services..."
	v.SearchEditor.SetIcon(icons.ActionSearch, components.IconPositionStart)

	// Initialize modals - display only, no save functionality
	v.statsModal = components.NewFormModal("Service Statistics")
	v.statsModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.statsModal.HideSaveButton = true

	v.pingModal = components.NewFormModal("Service Ping Results")
	v.pingModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.pingModal.HideSaveButton = true

	return v
}

func (v *ServicesView) SetApp(app App) {
	v.App = app
	v.Table.OnCopyFeedback = func(text string) {
		if v.App != nil {
			v.App.ShowToast("Copied: "+text, components.ToastTypeSuccess)
		}
	}
}

func (v *ServicesView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *ServicesView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.ServicesPageId,
		Title: "Services",
		Icon:  icons.ActionExtension,
	}
}

func (v *ServicesView) OnEnter() {
	v.Refresh()
}

func (v *ServicesView) FirstFocusTag() any {
	return v.SearchEditor.FocusTag()
}

func (v *ServicesView) LastFocusTag() any {
	return v.Paginator.NextClick
}

func (v *ServicesView) Refresh() {
	if v.App == nil || v.Loading {
		return
	}

	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		v.services = []*ServiceInfo{}
		v.EmptyState = true
		v.filterServices()
		return
	}

	v.Loading = true
	go func() {
		defer func() { v.Loading = false }()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		responses, err := client.ListServices(ctx)
		if err != nil {
			v.App.ShowToast("Failed to list services: "+err.Error(), components.ToastTypeError)
			return
		}

		newServices := make([]*ServiceInfo, 0, len(responses))
		for _, msg := range responses {
			var info struct {
				Name    string `json:"name"`
				Version string `json:"version"`
				ID      string `json:"id"`
			}
			if err := json.Unmarshal(msg.Data, &info); err == nil {
				newServices = append(newServices, &ServiceInfo{
					Name:    info.Name,
					Type:    info.Version,
					Status:  "Active",
					Subject: msg.Subject,
				})
			}
		}

		v.services = newServices
		v.EmptyState = len(newServices) == 0
		v.filterServices()
		if v.App != nil && v.App.GetCurrentPageID() == navigator.ServicesPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ServicesView) filterServices() {
	query := strings.ToLower(v.SearchEditor.GetText())
	v.filtered = make([]*ServiceInfo, 0)

	for _, s := range v.services {
		// Check search query
		if query != "" &&
			!strings.Contains(strings.ToLower(s.Name), query) &&
			!strings.Contains(strings.ToLower(s.Subject), query) &&
			!strings.Contains(strings.ToLower(s.Type), query) &&
			!strings.Contains(strings.ToLower(s.Group), query) &&
			!strings.Contains(strings.ToLower(s.Endpoint), query) {
			continue
		}

		// Check state filters - include if no filters selected OR if state matches a selected filter
		if !v.activeFilter.Selected && !v.idleFilter.Selected {
			// No filters selected, show all
			v.filtered = append(v.filtered, s)
		} else if v.activeFilter.Selected && s.Status == "Active" {
			v.filtered = append(v.filtered, s)
		} else if v.idleFilter.Selected && s.Status == "Idle" {
			v.filtered = append(v.filtered, s)
		}
		// If none of the above, service is filtered out
	}

	totalPages := (len(v.filtered) + v.PerPage - 1) / v.PerPage
	v.Paginator.SetTotalPages(totalPages)
	v.Table.ResetWidths()

	// Trigger UI refresh after filtering
	if v.App != nil {
		v.App.Invalidate()
	}
}

func (v *ServicesView) addSampleData() {
	v.services = []*ServiceInfo{
		{
			Name:       "OrderService",
			Subject:    "orders.>",
			Type:       "Service",
			Group:      "ecommerce",
			Endpoint:   "orders.new",
			Instances:  12,
			Calls:      125678,
			AvgLatency: "45ms",
			Throughput: "4500/s",
			Status:     "Active",
			Created:    "2024-01-10",
		},
		{
			Name:       "PaymentService",
			Subject:    "payments.>",
			Type:       "Service",
			Group:      "ecommerce",
			Endpoint:   "payments.process",
			Instances:  5,
			Calls:      234123,
			AvgLatency: "62ms",
			Throughput: "3200/s",
			Status:     "Active",
			Created:    "2024-01-12",
		},
		{
			Name:       "UserService",
			Subject:    "users.>",
			Type:       "Service",
			Group:      "accounts",
			Endpoint:   "users.info",
			Instances:  8,
			Calls:      456789,
			AvgLatency: "38ms",
			Throughput: "5800/s",
			Status:     "Idle",
			Created:    "2024-01-15",
		},
	}
	v.EmptyState = false
}

func (v *ServicesView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	for v.RefreshBtn.Clicked(gtx) {
		v.Refresh()
	}

	for v.resetBtn.Clicked(gtx) {
		if v.App != nil {
			v.App.ShowToast("Statistics reset", components.ToastTypeSuccess)
		}
	}

	// Handle Stats button click
	for v.statsBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			service := v.filtered[v.SelectedIdx]
			v.showServiceStats(service)
			log.Logger().Info().
				Str("service", service.Name).
				Str("action", "view_stats").
				Msg("Service stats viewed")
		}
	}

	// Handle Ping button click
	for v.pingBtn.Clicked(gtx) {
		v.pingAllServices()
		log.Logger().Info().
			Str("action", "ping_services").
			Int("total_services", len(v.services)).
			Msg("Service ping initiated")
	}

	// Only handle TAB navigation when no modal is open
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

	if v.SearchEditor.Changed() {
		v.LastFilterTime = gtx.Now
	}

	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
			v.filterServices()
			v.LastFilterTime = time.Time{}
		} else {
			gtx.Execute(op.InvalidateCmd{At: v.LastFilterTime.Add(300 * time.Millisecond)})
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

	paint.FillShape(gtx.Ops, th.Palette.Bg, clip.Rect{
		Max: gtx.Constraints.Max,
	}.Op())

	layout.UniformInset(unit.Dp(32)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		if v.isModalVisible() {
			cgtx = cgtx.Disabled()
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutHeader(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutActions(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutContent(ccgtx, th)
			}),
		)
	})

	// Render modals using layout.Stack
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutStatsModal(cgtx, th)
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutPingModal(cgtx, th)
		}),
	)
}

func (v *ServicesView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(24), "Services")
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

func (v *ServicesView) layoutActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	isSelected := v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered)

	refreshBtn := components.SecondaryButton(th, &v.RefreshBtn, icons.NavigationRefresh, components.IconPositionStart, "Refresh")
	resetBtn := components.SecondaryButton(th, &v.resetBtn, icons.NavigationRefresh, components.IconPositionStart, "Reset Stats")
	statsBtn := components.SecondaryButton(th, &v.statsBtn, icons.ActionInfo, components.IconPositionStart, "Stats")
	pingBtn := components.SecondaryButton(th, &v.pingBtn, icons.ActionSwapHoriz, components.IconPositionStart, "Ping")

	for v.activeFilter.Click.Clicked(gtx) {
		if v.activeFilter.Selected {
			v.activeFilter.SetSelected(false)
		} else {
			v.activeFilter.SetSelected(true)
			v.idleFilter.SetSelected(false)
		}
		v.filterServices()
	}

	for v.idleFilter.Click.Clicked(gtx) {
		if v.idleFilter.Selected {
			v.idleFilter.SetSelected(false)
		} else {
			v.idleFilter.SetSelected(true)
			v.activeFilter.SetSelected(false)
		}
		v.filterServices()
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return refreshBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return resetBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			return statsBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return pingBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.SearchEditor.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.activeFilter.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.idleFilter.Layout(cgtx, th)
		}),
	)
}

func (v *ServicesView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.EmptyState {
		return v.layoutEmptyState(gtx, th)
	}

	return v.Split.Layout(gtx, th,
		func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
					return v.layoutServicesTable(ccgtx, th)
				}),
			)
		},
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutServiceDetails(cgtx, th)
		},
	)
}

func (v *ServicesView) layoutServicesTable(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.Table.Rows = BuildTableRows(v.filtered, v.Paginator.CurrentPage, v.PerPage,
		func(service *ServiceInfo, idx int) components.TableRow {
			return components.TableRow{
				Values: []string{
					service.Name,
					service.Subject,
					fmt.Sprintf("%d", service.Instances),
					fmt.Sprintf("%d", service.Calls),
					service.AvgLatency,
					service.Throughput,
					service.Status,
				},
			}
		}, v.SelectedIdx)

	return components.Card{
		Title: "Registered Services",
	}.Layout(gtx, th, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				return v.Table.Layout(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.Paginator.Layout(ccgtx, th)
			}),
		)
	})
}

func (v *ServicesView) layoutServiceDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Select a service")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	service := v.filtered[v.SelectedIdx]

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.Card{
					Title: service.Name,
				}.Layout(ccgtx, th, func(cccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Subject", service.Subject)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Group", service.Group)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Endpoint", service.Endpoint)
						}),
					)
				})
			}),
		)
	})
}

func (v *ServicesView) handleTab(gtx layout.Context, shift bool) {
	tags := []any{
		&v.RefreshBtn,
		&v.resetBtn,
		&v.statsBtn,
		&v.pingBtn,
		v.SearchEditor.FocusTag(),
		v.activeFilter.FocusTag(),
		v.idleFilter.FocusTag(),
	}

	if !v.EmptyState && len(v.filtered) > 0 {
		tags = append(tags,
			v.Table.FocusTag(),
			v.Paginator.PrevClick,
			v.Paginator.NextClick,
		)
	}

	HandleTab(gtx, shift, tags, v.next, v.prev)
}

func (v *ServicesView) layoutEmptyState(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return components.EmptyState{
		Icon:    icons.ActionExtension,
		Title:   "No Services Found",
		Message: "Registered services will appear here.",
	}.Layout(gtx, th)
}

// layoutStatsModal renders the service stats modal
func (v *ServicesView) layoutStatsModal(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.selectedService != nil {
		v.statsModal.Title = fmt.Sprintf("Service: %s", v.selectedService.Name)
	} else {
		v.statsModal.Title = "Service Statistics"
	}
	v.statsModal.CustomContent = func(cgtx layout.Context, th *theme.Theme) layout.Dimensions {
		if v.selectedService == nil {
			return layout.Center.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(14), "No service selected")
				lbl.Color = th.TextColor
				return lbl.Layout(ccgtx)
			})
		}
		return v.layoutStatsContent(cgtx, th)
	}
	return v.statsModal.Layout(gtx, th)
}

// layoutStatsContent displays service information
func (v *ServicesView) layoutStatsContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	service := v.selectedService
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Name", service.Name)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Subject", service.Subject)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Type", service.Type)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Instances", fmt.Sprintf("%d", service.Instances))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Calls", fmt.Sprintf("%d", service.Calls))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Avg Latency", service.AvgLatency)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Throughput", service.Throughput)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Status", service.Status)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Created", service.Created)
		}),
	)
}

// layoutPingModal renders the ping results modal
func (v *ServicesView) layoutPingModal(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.pingModal.CustomContent = func(cgtx layout.Context, th *theme.Theme) layout.Dimensions {
		if v.pinging && len(v.pingResults) == 0 {
			return layout.Center.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(14), "Pinging services...")
				lbl.Color = th.TextColor
				return lbl.Layout(ccgtx)
			})
		}

		if len(v.pingResults) == 0 {
			return layout.Center.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(14), "No ping results")
				lbl.Color = th.TextColor
				return lbl.Layout(ccgtx)
			})
		}

		return v.layoutPingResults(cgtx, th)
	}
	return v.pingModal.Layout(gtx, th)
}

// layoutPingResults renders the ping results list
func (v *ServicesView) layoutPingResults(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		func() []layout.FlexChild {
			children := make([]layout.FlexChild, 0, len(v.pingResults)*2)
			for i, result := range v.pingResults {
				idx := i
				res := result
				children = append(children,
					layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
						return v.layoutPingResultRow(cgtx, th, res)
					}),
				)
				if idx < len(v.pingResults)-1 {
					children = append(children,
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					)
				}
			}
			return children
		}()...,
	)
}

// layoutPingResultRow renders a single ping result row
func (v *ServicesView) layoutPingResultRow(gtx layout.Context, th *theme.Theme, result PingResult) layout.Dimensions {
	var statusType components.StatusPillType
	if result.Status == "OK" {
		statusType = components.StatusPillSuccess
	} else {
		statusType = components.StatusPillError
	}

	return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.StatusPill{
					Text: result.Status,
					Type: statusType,
				}.Layout(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(14), result.ServiceName)
				lbl.Color = th.TextColor
				return lbl.Layout(ccgtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				if result.Status == "OK" {
					lbl := material.Label(th.Material(), unit.Sp(12), result.Latency.String())
					lbl.Color = th.TextColor
					return lbl.Layout(ccgtx)
				}
				lbl := material.Label(th.Material(), unit.Sp(12), result.Error)
				lbl.Color = th.TextColor
				return lbl.Layout(ccgtx)
			}),
		)
	})
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *ServicesView) HandleShortcuts(gtx layout.Context) bool {
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
func (v *ServicesView) GetShortcutsHelp() []shortcuts.Shortcut {
	return []shortcuts.Shortcut{
		shortcuts.Refresh(func() {}),
		shortcuts.CopyName(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.CopyRow(func() bool { return v.SelectedIdx >= 0 }, func() {}),
	}
}
