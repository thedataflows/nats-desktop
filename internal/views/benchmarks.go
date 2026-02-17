package views

import (
	"context"
	"fmt"
	"image"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/text"
	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"

	"github.com/thedataflows/nats-desktop/internal/models"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
)

type BenchmarksView struct {
	*BaseView

	benchmarkHistory []*BenchmarkResult
	filtered         []*BenchmarkResult

	// Extra buttons not in BaseView
	pubBtn     widget.Clickable
	subBtn     widget.Clickable
	jsBtn      widget.Clickable
	kvBtn      widget.Clickable
	serviceBtn widget.Clickable
	latencyBtn widget.Clickable
	stopBtn    widget.Clickable
	purgeBtn   widget.Clickable

	running            bool
	runningType        string
	progressMsg        atomic.Uint64
	progressTotal      atomic.Uint64
	lastProgressUpdate atomic.Int64 // UnixNano
	currentRunID       atomic.Int64

	configModal components.Modal
	modalTitle  string
	showModal   bool

	mu sync.Mutex

	// Benchmark configuration
	subjectInput *components.InputField
	countInput   *components.InputField
	rateInput    *components.InputField
	payloadInput *components.InputField
	clientsInput *components.InputField
	sizeInput    *components.InputField

	cancelFunc context.CancelFunc

	// Modal buttons
	SaveBtn   widget.Clickable
	CancelBtn widget.Clickable

	next, prev any
}

type BenchmarkManager interface {
	RunPublishBenchmark(ctx context.Context, config *models.BenchmarkConfig) (*models.BenchmarkResult, error)
	RunSubscribeBenchmark(ctx context.Context, config *models.BenchmarkConfig) (*models.BenchmarkResult, error)
	RunRequestReplyBenchmark(ctx context.Context, config *models.BenchmarkConfig) (*models.BenchmarkResult, error)
	RunJetStreamBenchmark(ctx context.Context, config *models.BenchmarkConfig) (*models.BenchmarkResult, error)
	RunKVBenchmark(ctx context.Context, config *models.BenchmarkConfig) (*models.BenchmarkResult, error)
}

type BenchmarkResult struct {
	Name       string
	Type       string
	Throughput string
	Latency    string
	Status     string
	Duration   string
	Date       string
	Details    string
}

func NewBenchmarksView(th *theme.Theme) *BenchmarksView {
	v := &BenchmarksView{
		BaseView: NewBaseView(
			[]string{"Name", "Type", "Throughput", "Latency", "Status", "Duration", "Date"},
			15,
		),
	}
	v.SearchEditor.Placeholder = "Search benchmarks..."
	v.SearchEditor.SetIcon(icons.ActionSearch, components.IconPositionStart)

	v.subjectInput = components.NewLabeledInputFieldWithPosition("Subject", "e.g. bench.pub", components.LabelPositionTop)
	v.countInput = components.NewLabeledInputFieldWithPosition("Count", "e.g. 100000", components.LabelPositionTop)
	v.rateInput = components.NewLabeledInputFieldWithPosition("Rate (msg/s)", "e.g. 10000", components.LabelPositionTop)
	v.clientsInput = components.NewLabeledInputFieldWithPosition("Clients", "e.g. 1", components.LabelPositionTop)
	v.sizeInput = components.NewLabeledInputFieldWithPosition("Size (bytes)", "e.g. 128", components.LabelPositionTop)

	v.subjectInput.SetText("benchmark.test")
	v.countInput.SetText("10000")
	v.rateInput.SetText("10000")
	v.clientsInput.SetText("1")
	v.sizeInput.SetText("128")

	return v
}

func (v *BenchmarksView) SetApp(app App) {
	v.App = app
	v.Table.OnCopyFeedback = func(text string) {
		if v.App != nil {
			v.App.ShowToast("Copied: "+text, components.ToastTypeSuccess)
		}
	}
}

func (v *BenchmarksView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *BenchmarksView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.BenchmarksPageId,
		Title: "Benchmarks",
		Icon:  icons.EditorInsertChart,
	}
}

func (v *BenchmarksView) OnEnter() {
	// Real data will populate benchmarkHistory
}

func (v *BenchmarksView) FirstFocusTag() any {
	return &v.pubBtn
}

func (v *BenchmarksView) LastFocusTag() any {
	return v.Paginator.NextClick
}

func (v *BenchmarksView) handleTab(gtx layout.Context, shift bool) {
	v.mu.Lock()
	modalOpen := v.showModal
	v.mu.Unlock()

	if modalOpen {
		// Modal tab navigation - cycle within modal only
		tags := []any{
			v.subjectInput.FocusTag(),
			v.countInput.FocusTag(),
			v.rateInput.FocusTag(),
			v.clientsInput.FocusTag(),
			v.sizeInput.FocusTag(),
			&v.CancelBtn,
			&v.SaveBtn,
		}
		HandleTab(gtx, shift, tags, nil, nil)
		return
	}

	tags := []any{
		&v.pubBtn,
		&v.subBtn,
		&v.jsBtn,
		&v.kvBtn,
		&v.serviceBtn,
		&v.latencyBtn,
		&v.purgeBtn,
		v.SearchEditor.FocusTag(),
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

func (v *BenchmarksView) showConfigModal(th *theme.Theme, benchmarkType string) {
	if v.App == nil {
		return
	}

	v.modalTitle = fmt.Sprintf("Configure %s Benchmark", benchmarkType)
	v.showModal = true
}

func (v *BenchmarksView) layoutConfigModal(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if !v.showModal {
		return layout.Dimensions{}
	}

	if v.configModal.Backdrop == nil {
		v.configModal.Backdrop = &widget.Clickable{}
	}

	v.configModal.MaxWidth = unit.Dp(400)
	return v.configModal.Layout(gtx, th, v.modalTitle, func(cgtx layout.Context) layout.Dimensions {
		// Limit input field widths
		maxWidth := cgtx.Dp(unit.Dp(300))
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				ccgtx.Constraints.Max.X = maxWidth
				return v.subjectInput.Layout(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				ccgtx.Constraints.Max.X = maxWidth
				return v.countInput.Layout(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				ccgtx.Constraints.Max.X = maxWidth
				return v.rateInput.Layout(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				ccgtx.Constraints.Max.X = maxWidth
				return v.clientsInput.Layout(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				ccgtx.Constraints.Max.X = maxWidth
				return v.sizeInput.Layout(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						btn := components.SecondaryButton(th, &v.CancelBtn, icons.NavigationClose, components.IconPositionStart, "Cancel")
						return btn.Layout(cccgtx, th)
					}),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						btn := components.Button(th, &v.SaveBtn, icons.AVPlayArrow, components.IconPositionStart, "Run")
						return btn.Layout(cccgtx, th)
					}),
				)
			}),
		)
	})
}

func (v *BenchmarksView) runBenchmark(benchmarkTypeFull string) {
	v.mu.Lock()
	if v.running {
		v.mu.Unlock()
		return
	}
	v.mu.Unlock()

	benchmarkType := strings.TrimSpace(strings.TrimPrefix(benchmarkTypeFull, "Configure"))
	benchmarkType = strings.TrimSpace(strings.TrimSuffix(benchmarkType, "Benchmark"))

	rawManager := v.App.GetBenchmarkManager()
	if rawManager == nil {
		v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
		return
	}

	manager, ok := rawManager.(BenchmarkManager)
	if !ok {
		v.App.ShowToast("Benchmark manager error", components.ToastTypeError)
		return
	}

	count, _ := strconv.Atoi(v.countInput.GetText())
	rate, _ := strconv.Atoi(v.rateInput.GetText())
	clients, _ := strconv.Atoi(v.clientsInput.GetText())
	size, _ := strconv.Atoi(v.sizeInput.GetText())

	if count <= 0 {
		count = 100000
	}
	if rate <= 0 {
		rate = 10000
	}
	if clients <= 0 {
		clients = 1
	}
	if size <= 0 {
		size = 128
	}

	subject := v.subjectInput.GetText()
	if subject == "" {
		v.App.ShowToast("Subject is required", components.ToastTypeError)
		return
	}

	v.mu.Lock()
	v.running = true
	v.runningType = benchmarkType
	v.progressMsg.Store(0)
	v.progressTotal.Store(uint64(count))
	newID := v.currentRunID.Add(1)
	thisRunID := newID
	v.lastProgressUpdate.Store(time.Now().UnixNano())
	v.mu.Unlock()

	if v.App != nil {
		v.App.Invalidate()
	}

	config := &models.BenchmarkConfig{
		Type:        benchmarkType,
		Subject:     subject,
		Count:       count,
		MessageRate: rate,
		Clients:     clients,
		Size:        size,
		OnProgress: func(messages uint64, total uint64) {
			if v.currentRunID.Load() != thisRunID {
				return
			}

			v.progressMsg.Store(messages)
			if total > 0 {
				v.progressTotal.Store(total)
			}

			now := time.Now()
			last := v.lastProgressUpdate.Load()
			if now.Sub(time.Unix(0, last)) < 50*time.Millisecond {
				return
			}

			v.lastProgressUpdate.Store(now.UnixNano())

			if v.App != nil {
				v.App.Invalidate()
			}
		},
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		v.mu.Lock()
		v.cancelFunc = cancel
		v.mu.Unlock()
		defer cancel()

		var res *models.BenchmarkResult
		var err error

		switch benchmarkType {
		case "Publish":
			res, err = manager.RunPublishBenchmark(ctx, config)
		case "Subscribe":
			res, err = manager.RunSubscribeBenchmark(ctx, config)
		case "Request-Reply":
			res, err = manager.RunRequestReplyBenchmark(ctx, config)
		case "Latency":
			// Latency is essentially a small payload Request-Reply
			if config.Size > 128 {
				config.Size = 128
			}
			res, err = manager.RunRequestReplyBenchmark(ctx, config)
		case "JetStream":
			res, err = manager.RunJetStreamBenchmark(ctx, config)
		case "KV":
			res, err = manager.RunKVBenchmark(ctx, config)
		default:
			err = fmt.Errorf("unknown benchmark type")
		}

		v.mu.Lock()
		if v.currentRunID.Load() == thisRunID {
			v.running = false
			v.progressMsg.Store(v.progressTotal.Load())
		}

		if err != nil {
			v.mu.Unlock()
			if v.App != nil {
				v.App.ShowToast(fmt.Sprintf("Benchmark failed: %v", err), components.ToastTypeError)
				v.App.Invalidate()
			}
			return
		}

		v.benchmarkHistory = append([]*BenchmarkResult{{
			Name:       fmt.Sprintf("%s Bench %d", benchmarkType, len(v.benchmarkHistory)+1),
			Type:       benchmarkType,
			Throughput: fmt.Sprintf("%.0f msg/s", res.MessagesPerSec),
			Latency:    fmt.Sprintf("%.2f ms P50", res.P50Latency.Seconds()*1000),
			Status:     "Complete",
			Duration:   res.Duration.Round(time.Millisecond).String(),
			Date:       time.Now().Format("2006-01-02 15:04"),
			Details:    fmt.Sprintf("Total: %d, Success: %d, Errors: %d\nMin: %v, Max: %v, Avg: %v\nP50: %v, P95: %v, P99: %v", res.TotalMessages, res.SuccessCount, res.ErrorCount, res.MinLatency, res.MaxLatency, res.AvgLatency, res.P50Latency, res.P95Latency, res.P99Latency),
		}}, v.benchmarkHistory...)
		v.EmptyState = false
		v.SelectedIdx = 0
		if len(v.Table.Rows) > 0 {
			v.Table.SelectedRow = 0
		}
		v.filterBenchmarksLocked()
		v.mu.Unlock()

		if v.App != nil {
			v.App.Invalidate()
			v.App.ShowToast("Benchmark complete", components.ToastTypeSuccess)
		}
	}()
}

func (v *BenchmarksView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	// 1. Process events without lock to avoid deadlock with state-locking methods
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift}, key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			if ke.Name == key.NameTab {
				v.handleTab(gtx, ke.Modifiers.Contain(key.ModShift))
			}
			if ke.Name == key.NameEscape {
				v.mu.Lock()
				v.showModal = false
				v.mu.Unlock()
			}
		}
	}

	if v.pubBtn.Clicked(gtx) {
		v.showConfigModal(th, "Publish")
	}

	if v.subBtn.Clicked(gtx) {
		v.showConfigModal(th, "Subscribe")
	}

	if v.jsBtn.Clicked(gtx) {
		v.showConfigModal(th, "JetStream")
	}

	if v.kvBtn.Clicked(gtx) {
		v.showConfigModal(th, "KV")
	}

	if v.serviceBtn.Clicked(gtx) {
		v.showConfigModal(th, "Request-Reply")
	}

	if v.latencyBtn.Clicked(gtx) {
		v.showConfigModal(th, "Latency")
	}

	if v.stopBtn.Clicked(gtx) {
		v.stopBenchmark()
	}

	if v.purgeBtn.Clicked(gtx) {
		v.mu.Lock()
		v.benchmarkHistory = make([]*BenchmarkResult, 0)
		v.EmptyState = true
		v.SelectedIdx = -1
		v.Table.SelectedRow = -1
		v.filterBenchmarksLocked()
		v.mu.Unlock()
	}

	if v.SaveBtn.Clicked(gtx) {
		v.mu.Lock()
		title := v.modalTitle
		v.showModal = false
		v.mu.Unlock()
		v.runBenchmark(strings.TrimPrefix(title, "Configure "))
	}

	if v.CancelBtn.Clicked(gtx) {
		v.mu.Lock()
		v.showModal = false
		v.mu.Unlock()
	}

	if v.configModal.Backdrop != nil {
		for v.configModal.Backdrop.Clicked(gtx) {
			v.mu.Lock()
			v.showModal = false
			v.mu.Unlock()
		}
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if v.SearchEditor.Changed() {
		v.LastFilterTime = gtx.Now
	}

	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
			v.filterBenchmarksLocked()
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

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
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
					layout.Flexed(1, func(cccgtx layout.Context) layout.Dimensions {
						return v.layoutContent(cccgtx, th)
					}),
				)
			})
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return v.layoutConfigModal(cgtx, th)
		}),
	)
}

func (v *BenchmarksView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(24), "Performance Benchmarks")
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

func (v *BenchmarksView) layoutActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					btn := components.Button(th, &v.pubBtn, icons.AVPlayArrow, components.IconPositionStart, "Pub")
					return btn.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					btn := components.Button(th, &v.subBtn, icons.AVPlayArrow, components.IconPositionStart, "Sub")
					return btn.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					btn := components.SecondaryButton(th, &v.jsBtn, icons.AVPlayArrow, components.IconPositionStart, "JetStream")
					return btn.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					btn := components.SecondaryButton(th, &v.kvBtn, icons.AVPlayArrow, components.IconPositionStart, "KV")
					return btn.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					btn := components.SecondaryButton(th, &v.serviceBtn, icons.AVPlayArrow, components.IconPositionStart, "Req/Rep")
					return btn.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					btn := components.SecondaryButton(th, &v.latencyBtn, icons.AVPlayArrow, components.IconPositionStart, "Latency")
					return btn.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					btn := components.SecondaryButton(th, &v.purgeBtn, icons.ContentDeleteSweep, components.IconPositionStart, "Purge")
					return btn.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
					return v.SearchEditor.Layout(ccgtx, th)
				}),
			)
		}),
	)
}

func (v *BenchmarksView) stopBenchmark() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cancelFunc != nil {
		v.cancelFunc()
		v.cancelFunc = nil
	}
	v.running = false
	if v.App != nil {
		v.App.Invalidate()
	}
}

func (v *BenchmarksView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.running {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					msg := v.progressMsg.Load()
					total := v.progressTotal.Load()
					progress := 0.0
					if total > 0 {
						progress = float64(msg) / float64(total)
					}
					res := material.Label(th.Material(), unit.Sp(18), fmt.Sprintf("Running %s Benchmark... %.1f%%", v.runningType, progress*100))
					res.Alignment = text.Middle
					return res.Layout(ccgtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					btn := components.Button(th, &v.stopBtn, icons.NavigationClose, components.IconPositionStart, "Stop Benchmark")
					return btn.Layout(ccgtx, th)
				}),
			)
		})
	}

	if v.EmptyState {
		return v.layoutEmptyState(gtx, th)
	}

	return v.Split.Layout(gtx, th,
		func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return v.layoutBenchmarkStats(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
				layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
					return v.layoutBenchmarkHistory(cccgtx, th)
				}),
			)
		},
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutBenchmarkDetails(cgtx, th)
		},
	)
}

func (v *BenchmarksView) layoutBenchmarkStats(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return components.Card{
		Title: "Performance Overview",
	}.Layout(gtx, th, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.StatCard{
					Title: "Total Runs",
					Value: fmt.Sprintf("%d", len(v.benchmarkHistory)),
				}.Layout(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.StatCard{
					Title: "Best Throughput",
					Value: "125k msg/s",
				}.Layout(ccgtx, th)
			}),
		)
	})
}

func (v *BenchmarksView) filterBenchmarksLocked() {
	v.filtered = make([]*BenchmarkResult, 0)
	query := strings.ToLower(v.SearchEditor.GetText())
	for _, b := range v.benchmarkHistory {
		if query != "" && !strings.Contains(strings.ToLower(b.Name), query) && !strings.Contains(strings.ToLower(b.Type), query) {
			continue
		}
		v.filtered = append(v.filtered, b)
	}
	totalPages := (len(v.filtered) + v.PerPage - 1) / v.PerPage
	v.Paginator.SetTotalPages(totalPages)
	v.Table.ResetWidths()
}

func (v *BenchmarksView) filterBenchmarks() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.filterBenchmarksLocked()
}

func (v *BenchmarksView) layoutBenchmarkHistory(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	startIdx := (v.Paginator.CurrentPage - 1) * v.PerPage
	endIdx := startIdx + v.PerPage
	if endIdx > len(v.filtered) {
		endIdx = len(v.filtered)
	}
	if startIdx < 0 || startIdx >= len(v.filtered) {
		startIdx = 0
		endIdx = 0
	}

	var pageHistory []*BenchmarkResult
	if endIdx > startIdx {
		pageHistory = v.filtered[startIdx:endIdx]
	}

	v.Table.Rows = make([]components.TableRow, len(pageHistory))
	for i, r := range pageHistory {
		v.Table.Rows[i] = components.TableRow{
			Values:   []string{r.Name, r.Type, r.Throughput, r.Latency, r.Status, r.Duration, r.Date},
			Selected: (startIdx + i) == v.SelectedIdx,
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.Table.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.Paginator.Layout(cgtx, th)
		}),
	)
}

func (v *BenchmarksView) layoutEmptyState(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return components.EmptyState{
		Icon:    icons.EditorInsertChart,
		Title:   "No Benchmarks Found",
		Message: "Run a benchmark to see your performance.",
	}.Layout(gtx, th)
}

func (v *BenchmarksView) layoutBenchmarkDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.benchmarkHistory) {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(16), "Select a benchmark")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	res := v.benchmarkHistory[v.SelectedIdx]
	return components.Card{
		Title: "Benchmark Details",
	}.Layout(gtx, th, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Name", res.Name)
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Type", res.Type)
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Throughput", res.Throughput)
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Latency", res.Latency)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(14), "Details")
				lbl.Color = th.TextColor
				lbl.Font.Weight = font.Bold
				return lbl.Layout(ccgtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(13), res.Details)
				lbl.Color = th.TextColor
				return lbl.Layout(ccgtx)
			}),
		)
	})
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *BenchmarksView) HandleShortcuts(gtx layout.Context) bool {
	// Check if any modal is visible first - view shortcuts disabled when modal is open
	if v.showModal {
		return false
	}

	// View-specific shortcuts - process only ONE event per frame
	ev, ok := gtx.Event(
		key.Filter{Name: key.NameSpace, Optional: key.ModShortcut},
		key.Filter{Name: key.Name("S"), Optional: key.ModShortcut},
	)
	if !ok {
		return false
	}

	if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
		switch {
		case ke.Name == key.NameSpace:
			// Space to start/stop benchmark
			if v.running {
				v.stopBtn.Click()
			}
			return true
		case ke.Name == key.Name("S") && ke.Modifiers.Contain(key.ModShortcut):
			// Ctrl+S to stop all
			if v.running {
				v.stopBtn.Click()
			}
			return true
		}
	}
	return false
}

// GetShortcutsHelp returns help text for this view's shortcuts
func (v *BenchmarksView) GetShortcutsHelp() []shortcuts.Shortcut {
	return []shortcuts.Shortcut{
		shortcuts.Toggle("Start/Stop", key.NameSpace, 0, func() bool { return v.running }, func() {}),
		shortcuts.Custom("Stop All", "Stop all benchmarks", key.Name("S"), key.ModShortcut, func() bool { return v.running }, func() {}),
		shortcuts.CopyName(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.CopyRow(func() bool { return v.SelectedIdx >= 0 }, func() {}),
	}
}
