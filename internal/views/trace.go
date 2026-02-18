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

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/navigator"
	"github.com/thedataflows/nats-desktop/internal/tracing"
	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
)

type TraceRow struct {
	Depth      int
	Kind       string
	Arrow      string
	Server     string
	Subject    string
	Details    string
	Latency    time.Duration
	Timestamp  time.Time
	HasError   bool
	EventType  string
	RawEvent   *server.MsgTraceEvent
	Ingress    *server.MsgTraceIngress
	Egress     *server.MsgTraceEgress
	JetStream  *server.MsgTraceJetStream
	SubjectMap *server.MsgTraceSubjectMapping
	StreamExp  *server.MsgTraceStreamExport
	ServiceImp *server.MsgTraceServiceImport
}

type TraceView struct {
	*BaseView

	subjectInput *components.InputField
	payloadInput *components.InputField
	headersInput *components.InputField
	timeoutInput *components.InputField

	deliverToggle        widget.Bool
	deliverCheck         components.CheckBoxStyle
	showTimestampsToggle widget.Bool
	showLatencyCheck     components.CheckBoxStyle

	startBtn widget.Clickable
	stopBtn  widget.Clickable
	clearBtn widget.Clickable

	tracing       bool
	rows          []*TraceRow
	baseTimestamp time.Time
	stats         struct {
		totalHops     int
		uniqueServers map[string]bool
		egressByKind  map[string]int
	}

	traceModal *components.FormModal

	cancelFunc context.CancelFunc

	next, prev any
	mu         sync.Mutex
	traceEvent *server.MsgTraceEvent
}

func NewTraceView(th *theme.Theme) *TraceView {
	v := &TraceView{
		BaseView: NewBaseView(
			[]string{"", "Kind", "Server", "Subject/Details", "Latency"},
			50,
		),
		rows: []*TraceRow{},
	}
	v.stats.uniqueServers = make(map[string]bool)
	v.stats.egressByKind = make(map[string]int)

	v.subjectInput = components.NewLabeledInputFieldWithPosition("", "e.g., orders.new", components.LabelPositionTop)
	v.subjectInput.SetIcon(icons.ActionLabel, components.IconPositionStart)

	v.payloadInput = components.NewLabeledInputFieldWithPosition("", "Message payload (optional)", components.LabelPositionTop)

	v.headersInput = components.NewLabeledInputFieldWithPosition("", "K:V format, comma-separated", components.LabelPositionTop)
	v.headersInput.SetIcon(icons.ContentContentCopy, components.IconPositionStart)

	v.timeoutInput = components.NewLabeledInputFieldWithPosition("", "e.g., 5s", components.LabelPositionTop)
	v.timeoutInput.SetIcon(icons.ActionSettings, components.IconPositionStart)
	v.timeoutInput.SetText("5s")

	v.deliverCheck = components.CheckBox(th.Material(), &v.deliverToggle, "Deliver to destination")
	v.deliverCheck.SetTheme(th)

	v.showLatencyCheck = components.CheckBox(th.Material(), &v.showTimestampsToggle, "Show latency")
	v.showLatencyCheck.SetTheme(th)
	v.showLatencyCheck.CheckBox.Value = true

	v.SearchEditor.Placeholder = "Search traces..."
	v.SearchEditor.SetIcon(icons.ActionSearch, components.IconPositionStart)

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

func (v *TraceView) SetApp(app App) {
	v.App = app
	v.Table.OnCopyFeedback = func(text string) {
		if v.App != nil {
			v.App.ShowToast("Copied: "+text, components.ToastTypeSuccess)
		}
	}
}

func (v *TraceView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *TraceView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.TracePageId,
		Title: "Trace",
		Icon:  icons.ActionVisibility,
	}
}

func (v *TraceView) OnEnter() {
}

func (v *TraceView) OnLeave() {
	v.stopTrace()
}

func (v *TraceView) FirstFocusTag() any {
	return v.subjectInput.FocusTag()
}

func (v *TraceView) LastFocusTag() any {
	return v.Paginator.NextClick
}

func (v *TraceView) Refresh() {
}

func (v *TraceView) runTrace() {
	if v.App == nil {
		return
	}

	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
		return
	}

	nc := client.Conn()
	if nc == nil {
		v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
		return
	}

	subject := strings.TrimSpace(v.subjectInput.GetText())
	if subject == "" {
		v.App.ShowToast("Subject is required", components.ToastTypeError)
		return
	}

	timeout := 5 * time.Second
	if timeoutStr := v.timeoutInput.GetText(); timeoutStr != "" {
		d, err := time.ParseDuration(timeoutStr)
		if err == nil && d > 0 {
			timeout = d
		}
	}

	v.mu.Lock()
	if v.tracing {
		v.mu.Unlock()
		return
	}
	v.tracing = true
	v.rows = nil
	v.traceEvent = nil
	v.stats.uniqueServers = make(map[string]bool)
	v.stats.egressByKind = make(map[string]int)
	v.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout+5*time.Second)
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

		msg := nats.NewMsg(subject)
		msg.Data = []byte(v.payloadInput.GetText())

		headersStr := v.headersInput.GetText()
		if headersStr != "" {
			pairs := strings.Split(headersStr, ",")
			for _, pair := range pairs {
				pair = strings.TrimSpace(pair)
				if pair == "" {
					continue
				}
				kv := strings.SplitN(pair, ":", 2)
				if len(kv) == 2 {
					msg.Header.Set(strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
				}
			}
		}

		event, err := tracing.TraceMsg(nc, msg, v.deliverToggle.Value, timeout)
		if err != nil && event == nil {
			if v.App != nil {
				v.App.ShowToast("Trace failed: "+err.Error(), components.ToastTypeError)
			}
			return
		}

		v.mu.Lock()
		v.traceEvent = event
		v.processTraceEvent(event)
		v.mu.Unlock()

		if v.App != nil {
			if err != nil {
				v.App.ShowToast("Trace completed with warnings: "+err.Error(), components.ToastTypeWarning)
			} else {
				v.App.ShowToast("Trace completed successfully", components.ToastTypeSuccess)
			}
			v.App.Invalidate()
		}

		select {
		case <-ctx.Done():
		default:
		}
	}()

	v.App.ShowToast("Tracing message to: "+subject, components.ToastTypeInfo)
}

func (v *TraceView) stopTrace() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.cancelFunc != nil {
		v.cancelFunc()
		v.cancelFunc = nil
	}
	v.tracing = false
}

func (v *TraceView) processTraceEvent(event *server.MsgTraceEvent) {
	if event == nil {
		return
	}
	v.rows = nil
	v.baseTimestamp = time.Time{}
	ingress := event.Ingress()
	if ingress != nil {
		v.baseTimestamp = ingress.Timestamp
	}
	v.flattenTraceEvent(event, 0)
}

func (v *TraceView) flattenTraceEvent(event *server.MsgTraceEvent, depth int) {
	if event == nil {
		return
	}

	ingress := event.Ingress()
	if ingress != nil {
		row := &TraceRow{
			Depth:     depth,
			Kind:      tracing.ServerKindString(ingress.Kind),
			Arrow:     tracing.KindToArrow(ingress.Kind),
			Server:    event.Server.Name,
			Subject:   ingress.Subject,
			Timestamp: ingress.Timestamp,
			HasError:  ingress.Error != "",
			Details:   ingress.Error,
			EventType: "ingress",
			RawEvent:  event,
			Ingress:   ingress,
		}
		if ingress.Account != "" {
			row.Details = fmt.Sprintf("account:%q", ingress.Account)
		}
		if event.Server.Version != "" {
			row.Details += fmt.Sprintf(" version:%q", event.Server.Version)
		}
		if event.Server.Cluster != "" {
			row.Details += fmt.Sprintf(" cluster:%q", event.Server.Cluster)
		}
		if ingress.Error != "" {
			row.HasError = true
			row.Details = fmt.Sprintf("Error: %s", ingress.Error)
		}
		v.rows = append(v.rows, row)
		v.stats.uniqueServers[event.Server.Name] = true
	}

	sm := event.SubjectMapping()
	if sm != nil {
		v.rows = append(v.rows, &TraceRow{
			Depth:      depth + 1,
			Kind:       "Mapping",
			Arrow:      "   |",
			Subject:    fmt.Sprintf("mapped to: %q", sm.MappedTo),
			Timestamp:  sm.Timestamp,
			EventType:  "mapping",
			SubjectMap: sm,
		})
	}

	for _, se := range event.StreamExports() {
		v.rows = append(v.rows, &TraceRow{
			Depth:     depth + 1,
			Kind:      "Stream Export",
			Arrow:     "   |",
			Subject:   fmt.Sprintf("to: %q account: %q", se.To, se.Account),
			Timestamp: se.Timestamp,
			EventType: "stream_export",
			StreamExp: se,
		})
	}

	for _, si := range event.ServiceImports() {
		v.rows = append(v.rows, &TraceRow{
			Depth:      depth + 1,
			Kind:       "Service Import",
			Arrow:      "   |",
			Subject:    fmt.Sprintf("from: %q to: %q account: %q", si.From, si.To, si.Account),
			Timestamp:  si.Timestamp,
			EventType:  "service_import",
			ServiceImp: si,
		})
	}

	js := event.JetStream()
	if js != nil {
		action := "stored"
		if js.NoInterest {
			action = "no interest"
		}
		details := fmt.Sprintf("action:%q stream:%q", action, js.Stream)
		if js.Subject != "" {
			details += fmt.Sprintf(" subject:%q", js.Subject)
		}
		hasError := js.Error != ""
		if hasError {
			details = fmt.Sprintf("Error: %s", js.Error)
		}
		v.rows = append(v.rows, &TraceRow{
			Depth:     depth + 1,
			Kind:      "JetStream",
			Arrow:     tracing.KindToArrow(server.JETSTREAM),
			Subject:   details,
			Timestamp: js.Timestamp,
			HasError:  hasError,
			EventType: "jetstream",
			JetStream: js,
		})
		v.stats.egressByKind["JetStream"]++
	}

	egresses := event.Egresses()
	if len(egresses) == 0 && ingress != nil && ingress.Kind == server.CLIENT && ingress.Error == "" {
		v.rows = append(v.rows, &TraceRow{
			Depth:     depth + 1,
			Kind:      "Result",
			Arrow:     "--X",
			Subject:   "No active interest",
			HasError:  true,
			EventType: "no_interest",
		})
	}

	for _, eg := range egresses {
		kindStr := tracing.ServerKindString(eg.Kind)
		details := ""
		if eg.Name != "" {
			details = fmt.Sprintf("%q", eg.Name)
		}
		if eg.Account != "" {
			details += fmt.Sprintf(" account:%q", eg.Account)
		}
		if eg.Subscription != "" {
			details += fmt.Sprintf(" subject:%q", eg.Subscription)
		}
		if eg.Queue != "" {
			details += fmt.Sprintf(" queue:%q", eg.Queue)
		}
		if eg.Error != "" {
			details = fmt.Sprintf("Error: %s", eg.Error)
		}

		v.rows = append(v.rows, &TraceRow{
			Depth:     depth + 1,
			Kind:      kindStr,
			Arrow:     tracing.KindToArrow(eg.Kind),
			Server:    eg.Name,
			Subject:   details,
			Timestamp: eg.Timestamp,
			HasError:  eg.Error != "",
			EventType: "egress",
			Egress:    eg,
		})
		v.stats.egressByKind[kindStr]++
		v.stats.totalHops++

		if eg.Link != nil {
			v.flattenTraceEvent(eg.Link, depth+2)
		}
	}
}

func (v *TraceView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.mu.Lock()
	tracing_ := v.tracing
	rows := make([]*TraceRow, len(v.rows))
	copy(rows, v.rows)
	traceEvent := v.traceEvent
	v.mu.Unlock()

	for v.startBtn.Clicked(gtx) {
		v.runTrace()
	}

	for v.stopBtn.Clicked(gtx) {
		v.stopTrace()
		if v.App != nil {
			v.App.ShowToast("Trace stopped", components.ToastTypeInfo)
		}
	}

	for v.clearBtn.Clicked(gtx) {
		v.mu.Lock()
		v.rows = nil
		v.traceEvent = nil
		v.stats.uniqueServers = make(map[string]bool)
		v.stats.egressByKind = make(map[string]int)
		v.mu.Unlock()
		if v.App != nil {
			v.App.ShowToast("Cleared", components.ToastTypeSuccess)
		}
	}

	if v.SearchEditor.Changed() {
		v.LastFilterTime = gtx.Now
	}

	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
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
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(rows) {
			v.traceModal.Show()
		}
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

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(32)).Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutHeader(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutConfig(cccgtx, th, tracing_)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
					layout.Flexed(1, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutTraceResults(cccgtx, th, rows, traceEvent)
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

func (v *TraceView) layoutConfig(gtx layout.Context, th *theme.Theme, tracingActive bool) layout.Dimensions {
	return components.Card{
		Title: "Configuration",
	}.Layout(gtx, th, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(ccgtx,
					layout.Flexed(0.35, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutInputField(cccgtx, th, "Subject*", v.subjectInput)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Flexed(0.35, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutInputField(cccgtx, th, "Payload", v.payloadInput)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Flexed(0.3, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutInputField(cccgtx, th, "Timeout", v.timeoutInput)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(ccgtx,
					layout.Flexed(0.5, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutInputField(cccgtx, th, "Headers (K:V, comma-separated)", v.headersInput)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Flexed(0.25, func(cccgtx layout.Context) layout.Dimensions {
						return v.deliverCheck.Layout(cccgtx)
					}),
					layout.Flexed(0.25, func(cccgtx layout.Context) layout.Dimensions {
						return v.showLatencyCheck.Layout(cccgtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						if tracingActive {
							btn := components.Button(th, &v.stopBtn, icons.AVPause, components.IconPositionStart, "Stop")
							return btn.Layout(cccgtx, th)
						}
						btn := components.Button(th, &v.startBtn, icons.AVPlayArrow, components.IconPositionStart, "Trace")
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

func (v *TraceView) layoutTraceResults(gtx layout.Context, th *theme.Theme, rows []*TraceRow, event *server.MsgTraceEvent) layout.Dimensions {
	if len(rows) == 0 {
		return components.EmptyState{
			Icon:    icons.ActionVisibility,
			Title:   "No Trace Results",
			Message: "Enter a subject and click 'Trace' to trace message delivery through the NATS network.",
		}.Layout(gtx, th)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutStats(cgtx, th, rows)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutLegend(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.layoutTraceTable(cgtx, th, rows)
		}),
	)
}

func (v *TraceView) layoutStats(gtx layout.Context, th *theme.Theme, rows []*TraceRow) layout.Dimensions {
	v.mu.Lock()
	serverCount := len(v.stats.uniqueServers)
	egressByKind := make(map[string]int)
	for k, v := range v.stats.egressByKind {
		egressByKind[k] = v
	}
	v.mu.Unlock()

	egressSummary := ""
	for k, c := range egressByKind {
		if egressSummary != "" {
			egressSummary += ", "
		}
		egressSummary += fmt.Sprintf("%s: %d", k, c)
	}
	if egressSummary == "" {
		egressSummary = "None"
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatCard{
				Title: "Total Rows",
				Value: fmt.Sprintf("%d", len(rows)),
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatCard{
				Title: "Servers",
				Value: fmt.Sprintf("%d", serverCount),
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatCard{
				Title: "Egress",
				Value: egressSummary,
			}.Layout(cgtx, th)
		}),
	)
}

func (v *TraceView) layoutLegend(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(12), "Legend: ")
			lbl.Color = th.SecondaryTextColor
			return lbl.Layout(cgtx)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatusPill{
				Text: "--C Client",
				Type: components.StatusPillNeutral,
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatusPill{
				Text: "--> Router",
				Type: components.StatusPillNeutral,
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatusPill{
				Text: "==> Gateway",
				Type: components.StatusPillNeutral,
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatusPill{
				Text: "~~> Leafnode",
				Type: components.StatusPillNeutral,
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatusPill{
				Text: "--J JetStream",
				Type: components.StatusPillSuccess,
			}.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return components.StatusPill{
				Text: "--X Error/No Interest",
				Type: components.StatusPillError,
			}.Layout(cgtx, th)
		}),
	)
}

func (v *TraceView) layoutTraceTable(gtx layout.Context, th *theme.Theme, rows []*TraceRow) layout.Dimensions {
	query := strings.ToLower(v.SearchEditor.GetText())
	var filtered []*TraceRow
	for _, r := range rows {
		if query == "" ||
			strings.Contains(strings.ToLower(r.Server), query) ||
			strings.Contains(strings.ToLower(r.Kind), query) ||
			strings.Contains(strings.ToLower(r.Subject), query) ||
			strings.Contains(strings.ToLower(r.Details), query) {
			filtered = append(filtered, r)
		}
	}

	startIdx := (v.Paginator.CurrentPage - 1) * v.PerPage
	endIdx := startIdx + v.PerPage
	if endIdx > len(filtered) {
		endIdx = len(filtered)
	}
	if startIdx < 0 || startIdx >= len(filtered) {
		startIdx = 0
		endIdx = 0
	}

	var pageRows []*TraceRow
	if endIdx > startIdx {
		pageRows = filtered[startIdx:endIdx]
	}

	v.Table.Rows = make([]components.TableRow, len(pageRows))
	for i, row := range pageRows {
		indent := strings.Repeat("  ", row.Depth)
		arrow := indent + row.Arrow

		latencyStr := ""
		if v.showTimestampsToggle.Value && !row.Timestamp.IsZero() && !v.baseTimestamp.IsZero() {
			latency := row.Timestamp.Sub(v.baseTimestamp)
			latencyStr = formatLatency(latency)
		}

		v.Table.Rows[i] = components.TableRow{
			Values: []string{
				arrow,
				row.Kind,
				row.Server,
				row.Subject,
				latencyStr,
			},
			Selected: (startIdx + i) == v.SelectedIdx,
		}
	}

	totalPages := (len(filtered) + v.PerPage - 1) / v.PerPage
	if totalPages < 1 {
		totalPages = 1
	}
	v.Paginator.SetTotalPages(totalPages)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.Table.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.Paginator.Layout(cgtx, th)
		}),
	)
}

func (v *TraceView) layoutTraceModalContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.mu.Lock()
	rows := make([]*TraceRow, len(v.rows))
	copy(rows, v.rows)
	v.mu.Unlock()

	if v.SelectedIdx < 0 || v.SelectedIdx >= len(rows) {
		return layout.Dimensions{}
	}

	row := rows[v.SelectedIdx]

	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Depth", fmt.Sprintf("%d", row.Depth))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Kind", row.Kind)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Server", row.Server)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Subject", row.Subject)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Details", row.Details)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if row.Timestamp.IsZero() {
				return layout.Dimensions{}
			}
			return layoutDetailRow(cgtx, th, "Timestamp", row.Timestamp.Format(time.RFC3339Nano))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !row.HasError {
				return layout.Dimensions{}
			}
			return layoutDetailRowColored(cgtx, th, "Error", row.Details, color.NRGBA{R: 244, G: 67, B: 54, A: 255})
		}),
	)

	return dims
}

func (v *TraceView) handleTab(gtx layout.Context, shift bool) {
	tags := []any{
		v.subjectInput.FocusTag(),
		v.payloadInput.FocusTag(),
		v.headersInput.FocusTag(),
		v.timeoutInput.FocusTag(),
	}

	if v.tracing {
		tags = append(tags, &v.stopBtn)
	} else {
		tags = append(tags, &v.startBtn)
	}

	tags = append(tags, &v.clearBtn, v.SearchEditor.FocusTag())

	if len(v.rows) > 0 {
		tags = append(tags,
			v.Table.FocusTag(),
			v.Paginator.PrevClick,
			v.Paginator.NextClick,
		)
	}

	HandleTab(gtx, shift, tags, v.next, v.prev)
}

func (v *TraceView) HandleShortcuts(gtx layout.Context) bool {
	return false
}

func (v *TraceView) GetShortcutsHelp() []shortcuts.Shortcut {
	return []shortcuts.Shortcut{
		shortcuts.CopyName(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.CopyRow(func() bool { return v.SelectedIdx >= 0 }, func() {}),
	}
}

func formatLatency(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fµs", float64(d.Nanoseconds())/1000.0)
	}
	return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/1e6)
}

func getLatencyColor(latency time.Duration) color.NRGBA {
	latencyMs := float64(latency.Nanoseconds()) / 1e6
	switch {
	case latencyMs < 5:
		return color.NRGBA{R: 76, G: 175, B: 80, A: 255}
	case latencyMs < 20:
		return color.NRGBA{R: 255, G: 193, B: 7, A: 255}
	default:
		return color.NRGBA{R: 244, G: 67, B: 54, A: 255}
	}
}
