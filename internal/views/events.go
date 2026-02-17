package views

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"sync"
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

	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
)

type EventsView struct {
	*BaseView

	events   []*EventEntry
	filtered []*EventEntry

	// Extra buttons not in BaseView
	filterBtn widget.Clickable
	clearBtn  widget.Clickable

	// Severity filter chips
	allFilter     *components.FilterChip
	infoFilter    *components.FilterChip
	warningFilter *components.FilterChip
	errorFilter   *components.FilterChip

	// Event type filter chips
	metricsFilter  *components.FilterChip
	advisoryFilter *components.FilterChip

	// Format dropdown
	formatDropDown *components.DropDown

	messageEditor *components.CodeEditor

	streamName  string
	totalEvents int64
	mu          sync.Mutex

	next, prev any
}

type EventEntry struct {
	ID            string
	Type          string
	Subject       string
	Message       string
	Time          string
	Severity      string
	EventCategory string // "metric", "advisory", "system"

	// Pre-lowercased for filtering speed
	idLower      string
	subjectLower string
	messageLower string
	typeLower    string
}

func NewEventsView(th *theme.Theme) *EventsView {
	v := &EventsView{
		BaseView: NewBaseView(
			[]string{"ID", "Type", "Subject", "Time", "Severity"},
			15,
		),
		// Severity filters
		allFilter:     components.NewFilterChip("All"),
		infoFilter:    components.NewFilterChip("Info"),
		warningFilter: components.NewFilterChip("Warning"),
		errorFilter:   components.NewFilterChip("Error"),
		// Event type filters
		metricsFilter:  components.NewFilterChip("Metrics"),
		advisoryFilter: components.NewFilterChip("Advisories"),
		// Data
		events:        []*EventEntry{},
		filtered:      []*EventEntry{},
		messageEditor: components.NewCodeEditor("", components.CodeLanguageJSON, th),
		streamName:    "EVENTS", // Default stream name for advisory events
	}

	// Format dropdown (JSON/Short/Full)
	v.formatDropDown = components.NewDropDown(
		components.NewDropDownOption("Full").WithValue("full").DefaultSelected(),
		components.NewDropDownOption("JSON").WithValue("json"),
		components.NewDropDownOption("Short").WithValue("short"),
	)
	v.formatDropDown.MinWidth = unit.Dp(100)

	v.SearchEditor.Placeholder = "Search events..."
	v.SearchEditor.SetIcon(icons.ActionSearch, components.IconPositionStart)

	v.allFilter.SetSelected(true)
	return v
}

func (v *EventsView) SetApp(app App) {
	v.App = app
	v.Table.OnCopyFeedback = func(text string) {
		if v.App != nil {
			v.App.ShowToast("Copied: "+text, components.ToastTypeSuccess)
		}
	}
}

func (v *EventsView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *EventsView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.EventsPageId,
		Title: "Events",
		Icon:  icons.ActionHistory,
	}
}

func (v *EventsView) OnEnter() {
	v.Refresh()
}

func (v *EventsView) FirstFocusTag() any {
	return &v.RefreshBtn
}

func (v *EventsView) LastFocusTag() any {
	v.mu.Lock()
	selectedRow := v.Table.SelectedRow
	count := len(v.filtered)
	v.mu.Unlock()

	if selectedRow >= 0 && selectedRow < count {
		return v.messageEditor.FocusTag()
	}
	if count > 0 {
		return v.Paginator.NextClick
	}
	return &v.errorFilter
}

func (v *EventsView) OnLeave() {
	// No longer needs cleanup as subscriptions are removed
}

func (v *EventsView) RefreshLocked() {
	// No-op - we don't perform local filtering for background updates anymore
}

func (v *EventsView) Refresh() {
	if v.App == nil || v.Loading {
		return
	}

	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		v.mu.Lock()
		v.events = []*EventEntry{}
		v.filtered = []*EventEntry{}
		v.EmptyState = true
		v.mu.Unlock()
		return
	}

	v.mu.Lock()
	v.Loading = true
	v.mu.Unlock()

	go func() {
		defer func() {
			v.mu.Lock()
			v.Loading = false
			v.mu.Unlock()
			if v.App != nil {
				v.App.Invalidate()
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// 1. Get statistics for total count
		s, err := client.GetJetStream().Stream(ctx, v.streamName)
		if err != nil {
			// Automatic discovery: find any stream that has advisories/events or is named similar
			streams, _ := client.ListStreams(ctx)
			for _, st := range streams {
				nameLower := strings.ToLower(st.Config.Name)
				matches := strings.Contains(nameLower, "event") || strings.Contains(nameLower, "advisory")

				if !matches {
					for _, sub := range st.Config.Subjects {
						subLower := strings.ToLower(sub)
						if strings.Contains(subLower, "advisory") || strings.Contains(subLower, "event") || strings.HasPrefix(sub, "$SYS.ADVISORY") || sub == ">" {
							matches = true
							break
						}
					}

					totalPages := (len(v.filtered) + v.PerPage - 1) / v.PerPage
					v.Paginator.SetTotalPages(totalPages)
					v.Table.ResetWidths()
				}

				if matches {
					v.streamName = st.Config.Name
					s, _ = client.GetJetStream().Stream(ctx, v.streamName)
					break
				}
			}
		}

		if s == nil {
			v.mu.Lock()
			v.EmptyState = true
			v.events = []*EventEntry{}
			v.filtered = []*EventEntry{}
			v.mu.Unlock()
			if v.App != nil {
				v.App.ShowToast("Event stream not found. Create a stream to capture advisories.", components.ToastTypeWarning)
			}
			return
		}

		info, err := s.Info(ctx)
		if err != nil {
			if v.App != nil {
				v.App.ShowToast("Failed to get stream info: "+err.Error(), components.ToastTypeError)
			}
			return
		}

		total := int64(info.State.Msgs)
		v.mu.Lock()
		v.totalEvents = total
		totalPages := (int(total) + v.PerPage - 1) / v.PerPage
		if totalPages == 0 {
			totalPages = 1
		}
		v.Paginator.SetTotalPages(totalPages)
		v.mu.Unlock()

		// 2. Load just the events for the current page
		// We calculate startSeq based on pagination (Newest first)
		if total == 0 {
			v.mu.Lock()
			v.EmptyState = true
			v.events = []*EventEntry{}
			v.filtered = []*EventEntry{}
			v.mu.Unlock()
			return
		}

		startSeq := info.State.LastSeq
		if v.Paginator.CurrentPage > 1 {
			offset := uint64((v.Paginator.CurrentPage - 1) * v.PerPage)
			if offset < startSeq {
				startSeq -= offset
			} else {
				startSeq = 0
			}
		}

		if startSeq == 0 {
			return
		}

		msgs, _, err := client.GetStreamMessages(ctx, v.streamName, v.PerPage, startSeq)
		if err != nil {
			if v.App != nil {
				v.App.ShowToast("Failed to fetch events: "+err.Error(), components.ToastTypeError)
			}
			return
		}

		newEvents := make([]*EventEntry, 0, len(msgs))
		for _, msg := range msgs {
			entry := &EventEntry{
				ID:       fmt.Sprintf("%d", msg.Sequence),
				Subject:  msg.Subject,
				Message:  string(msg.Data),
				Time:     msg.Time.Format("15:04:05"),
				Severity: "Info",
			}

			// Detect event category from subject
			subjectLower := strings.ToLower(msg.Subject)
			if strings.Contains(subjectLower, "$js.event.metric") {
				entry.EventCategory = "metric"
				entry.Type = "JetStream Metric"
			} else if strings.Contains(subjectLower, "$js.advisory") || strings.Contains(subjectLower, "$sys.advisory") {
				entry.EventCategory = "advisory"
				entry.Type = "Advisory"
			} else if strings.Contains(subjectLower, "$sys") {
				entry.EventCategory = "system"
				entry.Type = "System"
			} else {
				entry.EventCategory = "advisory"
				entry.Type = "Advisory"
			}

			if strings.Contains(msg.Subject, ".ERROR") {
				entry.Severity = "Error"
			} else if strings.Contains(msg.Subject, ".WARN") {
				entry.Severity = "Warning"
			}
			entry.idLower = strings.ToLower(entry.ID)
			entry.subjectLower = strings.ToLower(entry.Subject)
			entry.messageLower = strings.ToLower(entry.Message)
			entry.typeLower = strings.ToLower(entry.Type)
			newEvents = append(newEvents, entry)
		}

		v.mu.Lock()
		v.events = newEvents
		v.filterEventsLocked() // Apply local search filter to current page and update selection
		v.EmptyState = len(v.events) == 0
		v.mu.Unlock()
	}()
}

func (v *EventsView) filterEventsLocked() {
	v.filtered = make([]*EventEntry, 0)
	query := strings.ToLower(v.SearchEditor.GetText())
	for _, event := range v.events {
		// Lazily populate lowercase fields if they were skipped during background collection
		if event.idLower == "" {
			event.idLower = strings.ToLower(event.ID)
			event.subjectLower = strings.ToLower(event.Subject)
			event.messageLower = strings.ToLower(event.Message)
			event.typeLower = strings.ToLower(event.Type)
		}

		matchesSearch := query == "" ||
			strings.Contains(event.idLower, query) ||
			strings.Contains(event.subjectLower, query) ||
			strings.Contains(event.messageLower, query) ||
			strings.Contains(event.typeLower, query)

		if !matchesSearch {
			continue
		}

		// Check severity filter
		matchesSeverity := v.allFilter.Selected ||
			(v.infoFilter.Selected && event.Severity == "Info") ||
			(v.warningFilter.Selected && event.Severity == "Warning") ||
			(v.errorFilter.Selected && event.Severity == "Error")

		if !matchesSeverity {
			continue
		}

		// Check event type filter
		matchesType := !v.metricsFilter.Selected && !v.advisoryFilter.Selected
		if v.metricsFilter.Selected && event.EventCategory == "metric" {
			matchesType = true
		}
		if v.advisoryFilter.Selected && event.EventCategory == "advisory" {
			matchesType = true
		}

		if matchesType {
			v.filtered = append(v.filtered, event)
		}
	}

	if len(v.filtered) > 0 {
		if v.Table.SelectedRow < 0 || v.Table.SelectedRow >= len(v.filtered) {
			v.Table.SelectedRow = 0
		}
	} else {
		v.Table.SelectedRow = -1
	}
}

func (v *EventsView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.RestoreListFocus {
		v.RestoreListFocus = false
		gtx.Execute(key.FocusCmd{Tag: v.Table.FocusTag()})
	}

	for v.RefreshBtn.Clicked(gtx) {
		v.Refresh()
		if v.App != nil {
			v.App.ShowToast("Refreshed", components.ToastTypeInfo)
		}
	}

	for v.clearBtn.Clicked(gtx) {
		v.ConfirmModal.Title = "Clear Events"
		v.ConfirmModal.Content = "Are you sure you want to clear all events from NATS?"
		v.ConfirmModal.ReturnFocus = v.Table.FocusTag()
		v.ConfirmModal.SetOnClose(func() {
			v.RestoreListFocus = true
		})
		v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
			if option == "Confirm" {
				go func() {
					client := v.App.GetNatsClient()
					if client == nil {
						return
					}

					v.mu.Lock()
					streamName := v.streamName
					v.mu.Unlock()

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					if err := client.PurgeStream(ctx, streamName); err != nil {
						if v.App != nil {
							v.App.ShowToast("Failed to clear events: "+err.Error(), components.ToastTypeError)
						}
						return
					}

					v.mu.Lock()
					v.events = []*EventEntry{}
					v.filtered = []*EventEntry{}
					v.EmptyState = true
					v.totalEvents = 0
					v.Paginator.SetTotalPages(1)
					v.Paginator.CurrentPage = 1
					v.mu.Unlock()

					if v.App != nil {
						v.App.ShowToast("Events cleared", components.ToastTypeSuccess)
						v.App.Invalidate()
					}
				}()
			}
		})
		v.ConfirmModal.Show()
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

	if v.SearchEditor.Changed() {
		v.LastFilterTime = gtx.Now
	}

	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
			v.mu.Lock()
			v.filterEventsLocked()
			v.mu.Unlock()
			v.LastFilterTime = time.Time{}
		} else {
			gtx.Execute(op.InvalidateCmd{At: v.LastFilterTime.Add(300 * time.Millisecond)})
		}
	}

	for v.allFilter.Click.Clicked(gtx) {
		v.mu.Lock()
		v.allFilter.SetSelected(true)
		v.infoFilter.SetSelected(false)
		v.warningFilter.SetSelected(false)
		v.errorFilter.SetSelected(false)
		v.filterEventsLocked()
		v.mu.Unlock()
	}

	for v.infoFilter.Click.Clicked(gtx) {
		v.mu.Lock()
		v.infoFilter.SetSelected(true)
		v.allFilter.SetSelected(false)
		v.warningFilter.SetSelected(false)
		v.errorFilter.SetSelected(false)
		v.filterEventsLocked()
		v.mu.Unlock()
	}

	for v.warningFilter.Click.Clicked(gtx) {
		v.mu.Lock()
		v.warningFilter.SetSelected(true)
		v.allFilter.SetSelected(false)
		v.infoFilter.SetSelected(false)
		v.errorFilter.SetSelected(false)
		v.filterEventsLocked()
		v.mu.Unlock()
	}

	for v.errorFilter.Click.Clicked(gtx) {
		v.mu.Lock()
		v.errorFilter.SetSelected(true)
		v.allFilter.SetSelected(false)
		v.infoFilter.SetSelected(false)
		v.warningFilter.SetSelected(false)
		v.filterEventsLocked()
		v.mu.Unlock()
	}

	for v.metricsFilter.Click.Clicked(gtx) {
		v.mu.Lock()
		v.metricsFilter.SetSelected(!v.metricsFilter.Selected)
		v.advisoryFilter.SetSelected(false)
		v.filterEventsLocked()
		v.mu.Unlock()
	}

	for v.advisoryFilter.Click.Clicked(gtx) {
		v.mu.Lock()
		v.advisoryFilter.SetSelected(!v.advisoryFilter.Selected)
		v.metricsFilter.SetSelected(false)
		v.filterEventsLocked()
		v.mu.Unlock()
	}

	if v.formatDropDown.Changed() {
		v.mu.Lock()
		v.filterEventsLocked()
		v.mu.Unlock()
	}

	if v.Paginator.Next(gtx) {
		v.Paginator.NextPage()
		v.Refresh()
	}

	if v.Paginator.Prev(gtx) {
		v.Paginator.PrevPage()
		v.Refresh()
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
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Flexed(1, func(cccgtx layout.Context) layout.Dimensions {
						v.mu.Lock()
						loading := v.Loading
						isEmpty := v.EmptyState
						v.mu.Unlock()

						if loading {
							return layout.Center.Layout(cccgtx, func(gtx2 layout.Context) layout.Dimensions {
								return components.Spinner(gtx2, th)
							})
						}

						if isEmpty {
							return v.layoutEmptyState(cccgtx, th)
						}
						return v.layoutContent(cccgtx, th)
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
	)
}

func (v *EventsView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.mu.Lock()
	total := v.totalEvents
	stream := v.streamName
	v.mu.Unlock()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					header := material.Label(th.Material(), unit.Sp(24), "Event Stream")
					header.Color = th.TextColor
					return header.Layout(ccgtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					stats := fmt.Sprintf("Stream: %s | Total Events: %d", stream, total)
					lbl := material.Label(th.Material(), unit.Sp(13), stats)
					lbl.Color = th.SecondaryTextColor
					return lbl.Layout(ccgtx)
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

func (v *EventsView) layoutActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// First row: buttons, search (flexible), dropdown (fixed width)
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(cgtx,
				layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
					btn := components.SecondaryButton(th, &v.RefreshBtn, icons.NavigationRefresh, components.IconPositionStart, "Refresh")
					return btn.Layout(cgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
					btn := components.SecondaryButton(th, &v.clearBtn, icons.ActionDelete, components.IconPositionStart, "Clear")
					return btn.Layout(cgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				// Search expands to fill remaining space, minimum 50dp
				layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
					minWidth := cgtx.Dp(unit.Dp(50))
					if cgtx.Constraints.Min.X < minWidth {
						cgtx.Constraints.Min.X = minWidth
					}
					return v.SearchEditor.Layout(cgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				// Dropdown with fixed width
				layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
					cgtx.Constraints.Min.X = cgtx.Dp(unit.Dp(100))
					cgtx.Constraints.Max.X = cgtx.Dp(unit.Dp(100))
					return v.formatDropDown.Layout(cgtx, th)
				}),
			)
		}),
		// Second row: filter chips
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(cgtx,
				layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
					return v.allFilter.Layout(cgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
					return v.infoFilter.Layout(cgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
					return v.warningFilter.Layout(cgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
					return v.errorFilter.Layout(cgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
					return v.metricsFilter.Layout(cgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
					return v.advisoryFilter.Layout(cgtx, th)
				}),
			)
		}),
	)
}

func (v *EventsView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.EmptyState {
		return v.layoutEmptyState(gtx, th)
	}

	return v.Split.Layout(gtx, th,
		func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
					return v.layoutEventTable(ccgtx, th)
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
		},
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutEventDetails(cgtx, th)
		},
	)
}

// formatMessage formats the message content based on the selected output format
func (v *EventsView) formatMessage(message string) string {
	selectedFormat := v.formatDropDown.GetSelected()
	if selectedFormat == nil {
		return message
	}

	format := selectedFormat.GetValue()
	switch format {
	case "json":
		// Pretty print JSON
		var jsonData interface{}
		if err := json.Unmarshal([]byte(message), &jsonData); err == nil {
			prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
			if err == nil {
				return string(prettyJSON)
			}
		}
		// If JSON parsing fails, return original message
		return message
	case "short":
		// Truncate to first 200 characters
		if len(message) > 200 {
			return message[:200] + "..."
		}
		return message
	case "full":
		// Show complete message
		return message
	default:
		return message
	}
}

func (v *EventsView) layoutEventDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.mu.Lock()
	filtered := v.filtered
	selectedIdx := v.Table.SelectedRow

	if selectedIdx < 0 || selectedIdx >= len(filtered) {
		v.mu.Unlock()
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Select an event")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	event := filtered[selectedIdx]

	// Get the formatted message based on the selected format
	formattedMessage := v.formatMessage(event.Message)

	// Ensure the editor content is in sync with the selection
	if formattedMessage != v.messageEditor.GetCode() {
		// Set language based on format
		selectedFormat := v.formatDropDown.GetSelected()
		if selectedFormat != nil && selectedFormat.GetValue() == "json" {
			v.messageEditor.SetLanguage(components.CodeLanguageJSON)
		} else {
			v.messageEditor.SetLanguage(components.CodeLanguageText)
		}
		v.messageEditor.SetCode(formattedMessage)
	}
	v.mu.Unlock()

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.Card{
					Title: event.ID,
				}.Layout(ccgtx, th, func(cccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							stateType := components.StatusPillNeutral
							switch event.Severity {
							case "Info":
								stateType = components.StatusPillSuccess
							case "Warning":
								stateType = components.StatusPillWarning
							case "Error":
								stateType = components.StatusPillError
							}
							return components.StatusPill{
								Text: event.Severity,
								Type: stateType,
							}.Layout(c4gtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layout.Flex{Spacing: layout.SpaceBetween}.Layout(c4gtx,
								layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
									return components.StatCard{
										Title: "Type",
										Value: event.Type,
									}.Layout(c5gtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
									return components.StatCard{
										Title: "Time",
										Value: event.Time,
									}.Layout(c5gtx, th)
								}),
							)
						}),
					)
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				return components.Card{
					Title:    "Details",
					Flexible: true,
				}.Layout(ccgtx, th, func(cccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Subject", event.Subject)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							lbl := material.Label(th.Material(), unit.Sp(12), "Message")
							lbl.Color = th.SecondaryTextColor
							return lbl.Layout(c4gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
						layout.Flexed(1, func(c4gtx layout.Context) layout.Dimensions {
							return v.messageEditor.Layout(c4gtx, th)
						}),
					)
				})
			}),
		)
	})
}

func (v *EventsView) handleTab(gtx layout.Context, shift bool) {
	tags := []any{
		&v.RefreshBtn,
		&v.clearBtn,
		v.SearchEditor.FocusTag(),
		v.formatDropDown.FocusTag(),
		v.allFilter.FocusTag(),
		v.infoFilter.FocusTag(),
		v.warningFilter.FocusTag(),
		v.errorFilter.FocusTag(),
		v.metricsFilter.FocusTag(),
		v.advisoryFilter.FocusTag(),
	}

	v.mu.Lock()
	isEmpty := v.EmptyState
	count := len(v.filtered)
	selectedRow := v.Table.SelectedRow
	v.mu.Unlock()

	if !isEmpty && count > 0 {
		tags = append(tags,
			v.Table.FocusTag(),
			v.Paginator.PrevClick,
			v.Paginator.NextClick,
		)

		if selectedRow >= 0 && selectedRow < count {
			tags = append(tags, v.messageEditor.FocusTag())
		}
	}

	HandleTab(gtx, shift, tags, v.next, v.prev)
}

func (v *EventsView) layoutEventTable(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.mu.Lock()
	filtered := v.filtered
	selectedRow := v.Table.SelectedRow
	v.mu.Unlock()

	v.Table.Rows = make([]components.TableRow, len(filtered))
	for i, e := range filtered {
		v.Table.Rows[i] = components.TableRow{
			Values: []string{
				e.ID,
				e.Type,
				e.Subject,
				e.Time,
				e.Severity,
			},
			Selected: i == selectedRow,
		}
	}

	return v.Table.Layout(gtx, th)
}

func (v *EventsView) layoutEmptyState(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		return components.EmptyState{
			Icon:    icons.ActionHistory,
			Title:   "Not Connected",
			Message: "Connect to a NATS server to see events.",
		}.Layout(gtx, th)
	}

	return components.EmptyState{
		Icon:    icons.ActionHistory,
		Title:   "No Events Found",
		Message: "No JetStream stream found capturing events/advisories.\nTry creating a stream for subjects like $SYS.ADVISORY.>",
	}.Layout(gtx, th)
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *EventsView) HandleShortcuts(gtx layout.Context) bool {
	// TODO: Implement view-specific shortcuts
	return false
}

// GetShortcutsHelp returns help text for this view's shortcuts
func (v *EventsView) GetShortcutsHelp() []shortcuts.Shortcut {
	return []shortcuts.Shortcut{
		shortcuts.CopyName(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.CopyRow(func() bool { return v.SelectedIdx >= 0 }, func() {}),
	}
}
