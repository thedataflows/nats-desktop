package views

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"strings"
	"sync"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/nats-io/nats.go"
	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

// TraceEntry represents a single hop in the message trace
type TraceEntry struct {
	Hop       int
	Subject   string
	Server    string
	Timestamp time.Time
	Latency   time.Duration
	Type      string // "publish", "route", "gateway", "leafnode"
	Details   string
}

// TraceResult represents the complete trace result for a message
type TraceResult struct {
	MessageID    string
	StartTime    time.Time
	EndTime      time.Time
	TotalHops    int
	TotalLatency time.Duration
	Path         []TraceEntry
	Delivered    bool
}

// TraceView provides message tracing functionality through NATS network
type TraceView struct {
	*BaseView

	// Trace configuration
	subjectInput     *components.InputField
	maxMessagesInput *components.InputField
	timeoutInput     *components.InputField

	// Buttons
	startBtn widget.Clickable
	stopBtn  widget.Clickable
	clearBtn widget.Clickable

	// State
	tracing    bool
	traces     []*TraceEntry
	currentHop int

	// Modal
	traceModal *components.FormModal

	cancelFunc context.CancelFunc

	next, prev any
	mu         sync.Mutex
}

// NewTraceView creates a new TraceView
func NewTraceView(th *theme.Theme) *TraceView {
	v := &TraceView{
		BaseView: NewBaseView(
			[]string{"Hop", "Server", "Type", "Latency", "Subject"},
			10,
		),
		traces: []*TraceEntry{},
	}

	v.subjectInput = components.NewLabeledInputFieldWithPosition("", "e.g., foo.bar.>", components.LabelPositionTop)
	v.subjectInput.SetIcon(icons.ActionLabel, components.IconPositionStart)

	v.maxMessagesInput = components.NewLabeledInputFieldWithPosition("", "100", components.LabelPositionTop)
	v.maxMessagesInput.SetIcon(icons.ContentFilterList, components.IconPositionStart)

	v.timeoutInput = components.NewLabeledInputFieldWithPosition("", "e.g., 30s", components.LabelPositionTop)
	v.timeoutInput.SetIcon(icons.ActionSettings, components.IconPositionStart)

	v.SearchEditor.Placeholder = "Search traces..."
	v.SearchEditor.SetIcon(icons.ActionSearch, components.IconPositionStart)

	// Initialize trace modal - display only
	v.traceModal = components.NewFormModal("Trace Details")
	v.traceModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.layoutTraceModalContent(gtx, th)
	}
	v.traceModal.OnClose = func() {
		v.SelectedIdx = -1
	}
	v.traceModal.HideSaveButton = true

	return v
}

// SetApp sets the app reference
func (v *TraceView) SetApp(app App) {
	v.App = app
}

// SetNavigation sets the next and prev navigation tags
func (v *TraceView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

// Info returns navigation info for the Trace view
func (v *TraceView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.TracePageId,
		Title: "Trace",
		Icon:  icons.ActionVisibility,
	}
}

// OnEnter is called when the view is entered
func (v *TraceView) OnEnter() {
	// Nothing special to do on enter
}

// OnLeave is called when the view is left
func (v *TraceView) OnLeave() {
	v.stopTrace()
}

// FirstFocusTag returns the first focus tag
func (v *TraceView) FirstFocusTag() any {
	return v.subjectInput.FocusTag()
}

// LastFocusTag returns the last focus tag
func (v *TraceView) LastFocusTag() any {
	return v.Paginator.NextClick
}

// Refresh refreshes the view
func (v *TraceView) Refresh() {
	// Refresh is handled by the trace itself
}

// startTrace starts message tracing
func (v *TraceView) startTrace(subject string, maxMsgs int, timeout time.Duration) {
	if v.App == nil {
		return
	}

	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
		return
	}

	v.mu.Lock()
	if v.tracing {
		v.mu.Unlock()
		return
	}
	v.tracing = true
	v.traces = []*TraceEntry{}
	v.currentHop = 0
	v.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	v.cancelFunc = cancel

	go func() {
		defer func() {
			v.mu.Lock()
			v.tracing = false
			v.mu.Unlock()
			if v.App != nil {
				v.App.Invalidate()
			}
		}()

		// Subscribe to $SYS.> for system events
		sub, err := client.Subscribe("$SYS.>", func(msg *nats.Msg) {
			// In a real implementation, this would parse system messages
			// For demo purposes, we'll simulate traces
			_ = msg
		})
		if err != nil {
			if v.App != nil {
				v.App.ShowToast("Failed to subscribe to system events: "+err.Error(), components.ToastTypeError)
			}
			return
		}
		defer sub.Unsubscribe()

		// Simulate trace for demo
		v.simulateTrace(subject, maxMsgs)

		<-ctx.Done()

		if v.App != nil {
			v.App.ShowToast("Tracing stopped", components.ToastTypeInfo)
		}
	}()

	if v.App != nil {
		v.App.ShowToast("Started tracing messages on: "+subject, components.ToastTypeSuccess)
	}
}

// stopTrace stops message tracing
func (v *TraceView) stopTrace() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.cancelFunc != nil {
		v.cancelFunc()
		v.cancelFunc = nil
	}
	v.tracing = false

	if v.App != nil {
		v.App.ShowToast("Tracing stopped", components.ToastTypeInfo)
	}
}

// simulateTrace simulates message path for demo
func (v *TraceView) simulateTrace(subject string, maxMsgs int) {
	// Simulate a message trace path through the NATS network
	servers := []string{
		"nats-server-1",
		"nats-server-2",
		"nats-server-3",
	}

	types := []string{"publish", "route", "gateway", "leafnode"}
	baseTime := time.Now()
	baseLatency := time.Millisecond * 2

	for i := 0; i < maxMsgs && v.tracing; i++ {
		// Add a small delay between simulated hops
		time.Sleep(time.Millisecond * 100)

		v.mu.Lock()
		if !v.tracing {
			v.mu.Unlock()
			break
		}

		// Create trace entries for this message
		for j, server := range servers {
			entry := &TraceEntry{
				Hop:       j + 1,
				Subject:   subject,
				Server:    server,
				Timestamp: baseTime.Add(time.Duration(j) * baseLatency),
				Latency:   time.Duration(j+1) * baseLatency,
				Type:      types[j%len(types)],
				Details:   fmt.Sprintf("Message processed on %s", server),
			}
			v.traces = append(v.traces, entry)
			v.currentHop = entry.Hop
		}

		v.mu.Unlock()

		if v.App != nil {
			v.App.Invalidate()
		}

		baseTime = baseTime.Add(time.Millisecond * 50)
	}
}

// Layout implements the view layout
func (v *TraceView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.mu.Lock()
	tracing := v.tracing
	traces := make([]*TraceEntry, len(v.traces))
	copy(traces, v.traces)
	v.mu.Unlock()

	// Handle button clicks
	for v.startBtn.Clicked(gtx) {
		subject := v.subjectInput.GetText()
		if subject == "" {
			if v.App != nil {
				v.App.ShowToast("Subject is required", components.ToastTypeError)
			}
			continue
		}

		maxMsgs := 100
		if maxStr := v.maxMessagesInput.GetText(); maxStr != "" {
			fmt.Sscanf(maxStr, "%d", &maxMsgs)
		}
		if maxMsgs < 1 {
			maxMsgs = 1
		}
		if maxMsgs > 10000 {
			maxMsgs = 10000
		}

		timeout := 30 * time.Second
		if timeoutStr := v.timeoutInput.GetText(); timeoutStr != "" {
			d, err := time.ParseDuration(timeoutStr)
			if err == nil && d > 0 {
				timeout = d
			}
		}

		v.startTrace(subject, maxMsgs, timeout)
	}

	for v.stopBtn.Clicked(gtx) {
		v.stopTrace()
	}

	for v.clearBtn.Clicked(gtx) {
		v.mu.Lock()
		v.traces = []*TraceEntry{}
		v.currentHop = 0
		v.mu.Unlock()
		v.App.ShowToast("Traces cleared", components.ToastTypeSuccess)
	}

	// Handle search
	if v.SearchEditor.Changed() {
		v.LastFilterTime = gtx.Now
	}

	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
			v.filterTraces()
			v.LastFilterTime = time.Time{}
		} else {
			gtx.Execute(op.InvalidateCmd{At: v.LastFilterTime.Add(300 * time.Millisecond)})
		}
	}

	// Handle pagination
	if v.Paginator.Next(gtx) {
		v.Paginator.NextPage()
	}

	if v.Paginator.Prev(gtx) {
		v.Paginator.PrevPage()
	}

	// Handle table selection
	if v.Table.Clicked() || v.Table.SelectionChanged() {
		v.SelectedIdx = (v.Paginator.CurrentPage-1)*v.PerPage + v.Table.SelectedRow
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(traces) {
			v.traceModal.Show()
		}
	}

	// Handle TAB navigation
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			v.handleTab(gtx, ke.Modifiers.Contain(key.ModShift))
		}
	}

	// Background
	paint.FillShape(gtx.Ops, th.Palette.Bg, clip.Rect{
		Max: gtx.Constraints.Max,
	}.Op())

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(32)).Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutHeader(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutConfig(cccgtx, th, tracing)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
					layout.Flexed(1, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutTraceResults(cccgtx, th, traces)
					}),
				)
			})
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.ConfirmModal.IsVisible() {
				return v.ConfirmModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.traceModal.Visible {
				return v.traceModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
	)
}

// layoutHeader renders the header
func (v *TraceView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			title := material.Label(th.Material(), unit.Sp(24), "Message Trace")
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

// layoutConfig renders the trace configuration panel
func (v *TraceView) layoutConfig(gtx layout.Context, th *theme.Theme, tracing bool) layout.Dimensions {
	return components.Card{
		Title: "Configuration",
	}.Layout(gtx, th, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(ccgtx,
					layout.Flexed(0.4, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutInputField(cccgtx, th, "Subject Pattern", v.subjectInput)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					layout.Flexed(0.3, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutInputField(cccgtx, th, "Max Messages", v.maxMessagesInput)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					layout.Flexed(0.3, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutInputField(cccgtx, th, "Timeout", v.timeoutInput)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						if tracing {
							btn := components.Button(th, &v.stopBtn, icons.AVPause, components.IconPositionStart, "Stop")
							return btn.Layout(cccgtx, th)
						}
						btn := components.Button(th, &v.startBtn, icons.AVPlayArrow, components.IconPositionStart, "Start Trace")
						return btn.Layout(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						btn := components.SecondaryButton(th, &v.clearBtn, icons.ActionDelete, components.IconPositionStart, "Clear")
						return btn.Layout(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					layout.Flexed(1, func(cccgtx layout.Context) layout.Dimensions {
						return v.SearchEditor.Layout(cccgtx, th)
					}),
				)
			}),
		)
	})
}

// layoutInputField renders a labeled input field
func (v *TraceView) layoutInputField(gtx layout.Context, th *theme.Theme, label string, input *components.InputField) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), label)
			lbl.Font.Weight = font.Bold
			lbl.Color = th.TextColor
			return layout.Inset{Bottom: unit.Dp(4)}.Layout(cgtx, lbl.Layout)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return input.Layout(cgtx, th)
		}),
	)
}

// layoutTraceResults renders the trace results
func (v *TraceView) layoutTraceResults(gtx layout.Context, th *theme.Theme, traces []*TraceEntry) layout.Dimensions {
	if len(traces) == 0 {
		return components.EmptyState{
			Icon:    icons.ActionVisibility,
			Title:   "No Traces",
			Message: "Configure trace settings and click 'Start Trace' to begin monitoring message flow through the NATS network.",
		}.Layout(gtx, th)
	}

	// Calculate summary statistics
	var totalLatency time.Duration
	maxLatency := time.Duration(0)
	servers := make(map[string]bool)
	for _, t := range traces {
		totalLatency += t.Latency
		if t.Latency > maxLatency {
			maxLatency = t.Latency
		}
		servers[t.Server] = true
	}
	avgLatency := time.Duration(0)
	if len(traces) > 0 {
		avgLatency = totalLatency / time.Duration(len(traces))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutStats(cgtx, th, len(traces), len(servers), avgLatency, maxLatency)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.layoutTimeline(cgtx, th, traces)
		}),
	)
}

// layoutStats renders the summary statistics
func (v *TraceView) layoutStats(gtx layout.Context, th *theme.Theme, totalTraces, uniqueServers int, avgLatency, maxLatency time.Duration) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatCard{
				Title: "Total Hops",
				Value: fmt.Sprintf("%d", totalTraces),
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatCard{
				Title: "Servers",
				Value: fmt.Sprintf("%d", uniqueServers),
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatCard{
				Title: "Avg Latency",
				Value: formatLatency(avgLatency),
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatCard{
				Title: "Max Latency",
				Value: formatLatency(maxLatency),
			}.Layout(cgtx, th)
		}),
	)
}

// layoutTimeline renders the trace timeline visualization
func (v *TraceView) layoutTimeline(gtx layout.Context, th *theme.Theme, traces []*TraceEntry) layout.Dimensions {
	return components.Card{
		Title: "Trace Timeline",
	}.Layout(gtx, th, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutLegend(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutTraceTable(ccgtx, th, traces)
			}),
		)
	})
}

// layoutLegend renders the latency color legend
func (v *TraceView) layoutLegend(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatusPill{
				Text: "Fast (<5ms)",
				Type: components.StatusPillSuccess,
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatusPill{
				Text: "Medium (5-20ms)",
				Type: components.StatusPillWarning,
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatusPill{
				Text: "Slow (>20ms)",
				Type: components.StatusPillError,
			}.Layout(cgtx, th)
		}),
	)
}

// layoutTraceTable renders the trace entries as a table
func (v *TraceView) layoutTraceTable(gtx layout.Context, th *theme.Theme, traces []*TraceEntry) layout.Dimensions {
	// Calculate max latency for scaling
	maxLatency := time.Duration(0)
	for _, t := range traces {
		if t.Latency > maxLatency {
			maxLatency = t.Latency
		}
	}
	if maxLatency == 0 {
		maxLatency = time.Millisecond
	}

	// Filter and paginate
	query := v.SearchEditor.GetText()
	filtered := v.filterTraceEntries(traces, query)

	startIdx := (v.Paginator.CurrentPage - 1) * v.PerPage
	endIdx := startIdx + v.PerPage
	if endIdx > len(filtered) {
		endIdx = len(filtered)
	}
	if startIdx < 0 || startIdx >= len(filtered) {
		startIdx = 0
		endIdx = 0
	}

	var pageTraces []*TraceEntry
	if endIdx > startIdx {
		pageTraces = filtered[startIdx:endIdx]
	}

	// Build table rows
	v.Table.Rows = make([]components.TableRow, len(pageTraces))
	for i, entry := range pageTraces {
		v.Table.Rows[i] = components.TableRow{
			Values: []string{
				fmt.Sprintf("%d", entry.Hop),
				entry.Server,
				entry.Type,
				formatLatency(entry.Latency),
				entry.Subject,
			},
			Selected: (startIdx + i) == v.SelectedIdx,
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.Table.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(cgtx,
				layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
					// Latency bar visualization
					if len(pageTraces) > 0 {
						return v.layoutLatencyBar(ccgtx, th, pageTraces, maxLatency)
					}
					return layout.Dimensions{}
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return v.Paginator.Layout(ccgtx, th)
				}),
			)
		}),
	)
}

// layoutLatencyBar renders a visual latency bar
func (v *TraceView) layoutLatencyBar(gtx layout.Context, th *theme.Theme, traces []*TraceEntry, maxLatency time.Duration) layout.Dimensions {
	height := gtx.Dp(unit.Dp(8))
	maxWidth := float32(gtx.Constraints.Max.X)

	// Draw background
	paint.FillShape(gtx.Ops, th.TableBorderColor, clip.Rect{
		Max: image.Pt(gtx.Constraints.Max.X, height),
	}.Op())

	// Draw latency bars
	if len(traces) > 0 && maxLatency > 0 {
		totalWidth := float32(0)
		for _, t := range traces {
			// Calculate width proportionally
			width := (float32(t.Latency) / float32(maxLatency)) * maxWidth / float32(len(traces))
			if width < 1 {
				width = 1
			}

			// Determine color based on latency
			barColor := getLatencyColor(t.Latency)

			// Draw the bar segment
			paint.FillShape(gtx.Ops, barColor, clip.Rect{
				Min: image.Pt(int(totalWidth), 0),
				Max: image.Pt(int(totalWidth+width), height),
			}.Op())

			totalWidth += width
			if totalWidth >= maxWidth {
				break
			}
		}
	}

	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
}

// layoutTraceModalContent renders the modal content
func (v *TraceView) layoutTraceModalContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.mu.Lock()
	traces := make([]*TraceEntry, len(v.traces))
	copy(traces, v.traces)
	v.mu.Unlock()

	if v.SelectedIdx < 0 || v.SelectedIdx >= len(traces) {
		return layout.Dimensions{}
	}

	entry := traces[v.SelectedIdx]
	latencyColor := getLatencyColor(entry.Latency)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Hop", fmt.Sprintf("%d", entry.Hop))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Server", entry.Server)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Subject", entry.Subject)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Type", entry.Type)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRowColored(cgtx, th, "Latency", formatLatency(entry.Latency), latencyColor)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Timestamp", entry.Timestamp.Format(time.RFC3339))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Details", entry.Details)
		}),
	)
}

// getTypeIcon returns an icon based on trace type
func (v *TraceView) getTypeIcon(traceType string) interface{} {
	switch traceType {
	case "publish":
		return icons.ContentSend
	case "route":
		return icons.ActionSwapHoriz
	case "gateway":
		return icons.ActionSettingsEthernet
	case "leafnode":
		return icons.DeviceStorage
	default:
		return icons.ActionVisibility
	}
}

// filterTraceEntries filters traces based on search query
func (v *TraceView) filterTraceEntries(traces []*TraceEntry, query string) []*TraceEntry {
	if query == "" {
		return traces
	}

	filtered := make([]*TraceEntry, 0)
	query = strings.ToLower(query)
	for _, t := range traces {
		if strings.Contains(strings.ToLower(t.Subject), query) ||
			strings.Contains(strings.ToLower(t.Server), query) ||
			strings.Contains(strings.ToLower(t.Type), query) ||
			strings.Contains(strings.ToLower(t.Details), query) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// filterTraces filters and updates the view
func (v *TraceView) filterTraces() {
	if v.App != nil {
		v.App.Invalidate()
	}
}

// handleTab handles TAB key navigation
func (v *TraceView) handleTab(gtx layout.Context, shift bool) {
	tags := []any{
		v.subjectInput.FocusTag(),
		v.maxMessagesInput.FocusTag(),
		v.timeoutInput.FocusTag(),
	}

	if v.tracing {
		tags = append(tags, &v.stopBtn)
	} else {
		tags = append(tags, &v.startBtn)
	}

	tags = append(tags, &v.clearBtn, v.SearchEditor.FocusTag())

	if !v.EmptyState && len(v.traces) > 0 {
		tags = append(tags,
			v.Table.FocusTag(),
			v.Paginator.PrevClick,
			v.Paginator.NextClick,
		)
	}

	HandleTab(gtx, shift, tags, v.next, v.prev)
}

// formatLatency formats a duration for display
func formatLatency(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fµs", float64(d.Nanoseconds())/1000.0)
	}
	return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/1e6)
}

// getLatencyColor returns the color based on latency
func getLatencyColor(latency time.Duration) color.NRGBA {
	latencyMs := float64(latency.Nanoseconds()) / 1e6
	switch {
	case latencyMs < 5:
		return color.NRGBA{R: 76, G: 175, B: 80, A: 255} // Green
	case latencyMs < 20:
		return color.NRGBA{R: 255, G: 193, B: 7, A: 255} // Yellow
	default:
		return color.NRGBA{R: 244, G: 67, B: 54, A: 255} // Red
	}
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *TraceView) HandleShortcuts(gtx layout.Context) bool {
	// TODO: Implement view-specific shortcuts
	return false
}

// GetShortcutsHelp returns help text for this view's shortcuts
func (v *TraceView) GetShortcutsHelp() []shortcuts.Shortcut {
	return nil
}
