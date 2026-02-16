package views

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"

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

	"github.com/nats-io/nats.go/jetstream"
	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

const counterKVBucket = "COUNTERS"

// CounterView manages distributed counters in NATS
type CounterView struct {
	*BaseView

	// Counters
	counters []*CounterInfo
	filtered []*CounterInfo

	// Buttons
	createBtn widget.Clickable
	deleteBtn widget.Clickable
	getBtn    widget.Clickable
	incrBtn   widget.Clickable
	decrBtn   widget.Clickable

	// Modals
	createModal *components.FormModal
	valueModal  *components.FormModal

	// Inputs
	counterNameInput  *components.InputField
	initialValueInput *components.InputField
	incrementInput    *components.InputField

	// Current value display
	currentCounter string
	currentValue   int64

	// Navigation
	next, prev any
}

// CounterInfo represents a distributed counter
type CounterInfo struct {
	Name    string
	Value   int64
	Subject string
	Created string
	Updated string
}

// counterData represents the JSON structure stored in KV
type counterData struct {
	Value   int64  `json:"value"`
	Updated string `json:"updated"`
	Created string `json:"created"`
}

// NewCounterView creates a new counter view
func NewCounterView(th *theme.Theme) *CounterView {
	v := &CounterView{
		BaseView: NewBaseView(
			[]string{"Name", "Value", "Subject", "Created"},
			15,
		),
		counters:          []*CounterInfo{},
		filtered:          []*CounterInfo{},
		counterNameInput:  components.NewLabeledInputFieldWithPosition("Counter name", "", components.LabelPositionTop),
		initialValueInput: components.NewLabeledInputFieldWithPosition("Initial value", "Optional", components.LabelPositionTop),
		incrementInput:    components.NewLabeledInputFieldWithPosition("Increment by", "", components.LabelPositionTop),
	}

	// Set default increment value
	v.incrementInput.SetText("1")

	// Initialize create modal
	v.createModal = components.NewFormModal("Create Counter")
	v.createModal.MaxWidth = unit.Dp(400)
	v.createModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(c6gtx layout.Context) layout.Dimensions {
				return v.counterNameInput.Layout(c6gtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(c6gtx layout.Context) layout.Dimensions {
				return v.initialValueInput.Layout(c6gtx, th)
			}),
		)
	}
	v.createModal.CustomFocusTags = []event.Tag{
		v.counterNameInput.FocusTag(),
		v.initialValueInput.FocusTag(),
	}
	v.createModal.OnSave = func() bool {
		name := v.counterNameInput.GetText()
		if name == "" {
			if v.App != nil {
				v.App.ShowToast("Counter name is required", components.ToastTypeError)
			}
			return false
		}
		initialVal, _ := strconv.ParseInt(v.initialValueInput.GetText(), 10, 64)
		v.createCounter(name, initialVal)
		v.RestoreListFocus = true
		return true
	}
	v.createModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.createModal.ReturnFocus = v.Table.FocusTag()

	// Initialize value modal - display only
	v.valueModal = components.NewFormModal("Counter Value")
	v.valueModal.MaxWidth = unit.Dp(350)
	v.valueModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return layout.Center.Layout(gtx, func(c5gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(c5gtx,
				layout.Rigid(func(c6gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th.Material(), unit.Sp(14), "Current Value")
					lbl.Color = th.TextColor
					return lbl.Layout(c6gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(c6gtx layout.Context) layout.Dimensions {
					val := material.Label(th.Material(), unit.Sp(48), fmt.Sprintf("%d", v.currentValue))
					val.Color = th.Palette.ContrastBg
					return val.Layout(c6gtx)
				}),
			)
		})
	}
	v.valueModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.valueModal.ReturnFocus = v.Table.FocusTag()
	v.valueModal.HideSaveButton = true

	return v
}

// SetApp sets the app reference
func (v *CounterView) SetApp(app App) {
	v.App = app
}

// SetNavigation sets the next and prev navigation tags
func (v *CounterView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

// Info returns navigation info
func (v *CounterView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.CounterPageId,
		Title: "Counters",
		Icon:  icons.ActionLabel,
	}
}

// OnEnter is called when the view is entered
func (v *CounterView) OnEnter() {
	v.Refresh()
}

// FirstFocusTag returns the first focus tag
func (v *CounterView) FirstFocusTag() any {
	return &v.createBtn
}

// LastFocusTag returns the last focus tag
func (v *CounterView) LastFocusTag() any {
	return v.Paginator.NextClick
}

// Refresh loads counters from NATS
func (v *CounterView) Refresh() {
	if v.App == nil || v.Loading {
		return
	}

	client := v.App.NATS()
	if client == nil || !client.IsConnected() {
		v.counters = []*CounterInfo{}
		v.filtered = []*CounterInfo{}
		v.EmptyState = true
		return
	}

	v.Loading = true
	go func() {
		defer func() {
			v.Loading = false
			if v.App != nil && v.App.GetCurrentPageID() == navigator.CounterPageId {
				v.App.Invalidate()
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Get or create the counters bucket
		kv, err := client.GetJetStream().KeyValue(ctx, counterKVBucket)
		if err != nil {
			// Bucket doesn't exist, try to create it
			kv, err = client.GetJetStream().CreateKeyValue(ctx, jetstream.KeyValueConfig{
				Bucket: counterKVBucket,
			})
			if err != nil {
				v.App.ShowToast("Failed to access counters bucket: "+err.Error(), components.ToastTypeError)
				return
			}
		}

		// List all keys in the bucket
		keys, err := kv.Keys(ctx)
		if err != nil {
			v.App.ShowToast("Failed to list counters: "+err.Error(), components.ToastTypeError)
			return
		}

		newCounters := make([]*CounterInfo, 0, len(keys))
		for _, key := range keys {
			entry, err := kv.Get(ctx, key)
			if err != nil {
				continue
			}

			var data counterData
			if err := json.Unmarshal(entry.Value(), &data); err != nil {
				// Try to parse as plain integer for backwards compatibility
				if val, err := strconv.ParseInt(string(entry.Value()), 10, 64); err == nil {
					data.Value = val
					data.Updated = entry.Created().Format(time.RFC3339)
					data.Created = entry.Created().Format(time.RFC3339)
				}
			}

			newCounters = append(newCounters, &CounterInfo{
				Name:    key,
				Value:   data.Value,
				Subject: fmt.Sprintf("$KV.%s.%s", counterKVBucket, key),
				Created: data.Created,
				Updated: data.Updated,
			})
		}

		v.counters = newCounters
		v.EmptyState = len(newCounters) == 0
		v.filterCounters()
	}()
}

// filterCounters filters counters based on search query
func (v *CounterView) filterCounters() {
	query := strings.ToLower(v.SearchEditor.GetText())
	if query == "" {
		v.filtered = v.counters
	} else {
		v.filtered = make([]*CounterInfo, 0)
		for _, c := range v.counters {
			if strings.Contains(strings.ToLower(c.Name), query) {
				v.filtered = append(v.filtered, c)
			}
		}
	}
	totalPages := (len(v.filtered) + v.PerPage - 1) / v.PerPage
	v.Paginator.SetTotalPages(totalPages)
	v.Table.ResetWidths()
}

// createCounter creates a new counter
func (v *CounterView) createCounter(name string, initial int64) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := v.App.NATS()
		if client == nil || !client.IsConnected() {
			v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
			return
		}

		// Get or create the counters bucket
		kv, err := client.GetJetStream().KeyValue(ctx, counterKVBucket)
		if err != nil {
			kv, err = client.GetJetStream().CreateKeyValue(ctx, jetstream.KeyValueConfig{
				Bucket: counterKVBucket,
			})
			if err != nil {
				v.App.ShowToast("Failed to create counters bucket: "+err.Error(), components.ToastTypeError)
				return
			}
		}

		// Check if counter already exists
		_, err = kv.Get(ctx, name)
		if err == nil {
			v.App.ShowToast("Counter already exists: "+name, components.ToastTypeError)
			return
		}

		// Create counter with initial value
		data := counterData{
			Value:   initial,
			Created: time.Now().Format(time.RFC3339),
			Updated: time.Now().Format(time.RFC3339),
		}

		jsonData, err := json.Marshal(data)
		if err != nil {
			v.App.ShowToast("Failed to encode counter data: "+err.Error(), components.ToastTypeError)
			return
		}

		_, err = kv.Create(ctx, name, jsonData)
		if err != nil {
			v.App.ShowToast("Failed to create counter: "+err.Error(), components.ToastTypeError)
			log.Logger().Error().
				Str("counter", name).
				Int64("initial_value", initial).
				Err(err).
				Msg("Counter creation failed")
			return
		}

		v.App.ShowToast("Counter created successfully", components.ToastTypeSuccess)
		log.Logger().Info().
			Str("counter", name).
			Int64("initial_value", initial).
			Msg("Counter created")
		v.Refresh()
	}()
}

// deleteCounter deletes a counter
func (v *CounterView) deleteCounter(name string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := v.App.NATS()
		if client == nil || !client.IsConnected() {
			v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
			return
		}

		kv, err := client.GetJetStream().KeyValue(ctx, counterKVBucket)
		if err != nil {
			v.App.ShowToast("Failed to access counters bucket: "+err.Error(), components.ToastTypeError)
			return
		}

		err = kv.Delete(ctx, name)
		if err != nil {
			v.App.ShowToast("Failed to delete counter: "+err.Error(), components.ToastTypeError)
			return
		}

		v.App.ShowToast("Counter deleted", components.ToastTypeSuccess)
		v.SelectedIdx = -1
		v.Refresh()
	}()
}

// getCounterValue gets the current value of a counter
func (v *CounterView) getCounterValue(name string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := v.App.NATS()
		if client == nil || !client.IsConnected() {
			v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
			return
		}

		kv, err := client.GetJetStream().KeyValue(ctx, counterKVBucket)
		if err != nil {
			v.App.ShowToast("Failed to access counters bucket: "+err.Error(), components.ToastTypeError)
			return
		}

		entry, err := kv.Get(ctx, name)
		if err != nil {
			v.App.ShowToast("Failed to get counter value: "+err.Error(), components.ToastTypeError)
			return
		}

		var data counterData
		if err := json.Unmarshal(entry.Value(), &data); err != nil {
			if val, err := strconv.ParseInt(string(entry.Value()), 10, 64); err == nil {
				data.Value = val
			} else {
				v.App.ShowToast("Failed to parse counter value", components.ToastTypeError)
				return
			}
		}

		v.currentCounter = name
		v.currentValue = data.Value
		v.valueModal.Title = fmt.Sprintf("Counter: %s", name)
		v.valueModal.Show()

		if v.App != nil {
			v.App.Invalidate()
		}
	}()
}

// incrementCounter increments a counter by the specified amount
func (v *CounterView) incrementCounter(name string, delta int64) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := v.App.NATS()
		if client == nil || !client.IsConnected() {
			v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
			return
		}

		kv, err := client.GetJetStream().KeyValue(ctx, counterKVBucket)
		if err != nil {
			v.App.ShowToast("Failed to access counters bucket: "+err.Error(), components.ToastTypeError)
			return
		}

		// Get current value
		entry, err := kv.Get(ctx, name)
		if err != nil {
			v.App.ShowToast("Failed to get counter: "+err.Error(), components.ToastTypeError)
			return
		}

		var data counterData
		if err := json.Unmarshal(entry.Value(), &data); err != nil {
			if val, err := strconv.ParseInt(string(entry.Value()), 10, 64); err == nil {
				data.Value = val
			} else {
				v.App.ShowToast("Failed to parse counter value", components.ToastTypeError)
				return
			}
		}

		// Update value
		data.Value += delta
		data.Updated = time.Now().Format(time.RFC3339)

		jsonData, err := json.Marshal(data)
		if err != nil {
			v.App.ShowToast("Failed to encode counter data: "+err.Error(), components.ToastTypeError)
			return
		}

		_, err = kv.Update(ctx, name, jsonData, entry.Revision())
		if err != nil {
			v.App.ShowToast("Failed to update counter: "+err.Error(), components.ToastTypeError)
			return
		}

		action := "incremented"
		if delta < 0 {
			action = "decremented"
		}
		v.App.ShowToast(fmt.Sprintf("Counter %s by %d", action, delta), components.ToastTypeSuccess)
		v.Refresh()
	}()
}

// Layout implements the view layout
func (v *CounterView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	// Handle deferred filter
	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
			v.filterCounters()
			v.LastFilterTime = time.Time{}
		} else {
			gtx.Execute(op.InvalidateCmd{At: v.LastFilterTime.Add(300 * time.Millisecond)})
		}
	}

	// Handle button clicks
	for v.createBtn.Clicked(gtx) {
		v.createModal.Show()
		v.counterNameInput.SetText("")
		v.initialValueInput.SetText("0")
	}

	for v.RefreshBtn.Clicked(gtx) {
		v.Refresh()
	}

	for v.DeleteBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.promptCounterDelete(v.filtered[v.SelectedIdx].Name)
		}
	}

	for v.getBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.getCounterValue(v.filtered[v.SelectedIdx].Name)
		}
	}

	for v.incrBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			delta, _ := strconv.ParseInt(v.incrementInput.GetText(), 10, 64)
			if delta == 0 {
				delta = 1
			}
			v.incrementCounter(v.filtered[v.SelectedIdx].Name, delta)
		}
	}

	for v.decrBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			delta, _ := strconv.ParseInt(v.incrementInput.GetText(), 10, 64)
			if delta == 0 {
				delta = 1
			}
			v.incrementCounter(v.filtered[v.SelectedIdx].Name, -delta)
		}
	}

	// Handle search input changes
	if v.SearchEditor.Changed() {
		v.LastFilterTime = gtx.Now
	}

	// Handle pagination
	if v.Paginator.Next(gtx) {
		v.Paginator.NextPage()
	}

	if v.Paginator.Prev(gtx) {
		v.Paginator.PrevPage()
	}

	// Handle table selection
	clicked := v.Table.Clicked()
	doubleClicked := v.Table.DoubleClicked()
	if clicked || doubleClicked {
		newIdx := (v.Paginator.CurrentPage-1)*v.PerPage + v.Table.SelectedRow
		if doubleClicked {
			if newIdx >= 0 && newIdx < len(v.filtered) {
				v.getCounterValue(v.filtered[newIdx].Name)
			}
		}
		v.SelectedIdx = newIdx
	}

	if v.Table.SelectionChanged() {
		v.SelectedIdx = (v.Paginator.CurrentPage-1)*v.PerPage + v.Table.SelectedRow
	}

	// Handle TAB navigation
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

	// Background
	paint.FillShape(gtx.Ops, th.Palette.Bg, clip.Rect{
		Max: gtx.Constraints.Max,
	}.Op())

	// Main layout
	return layout.UniformInset(unit.Dp(32)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		if v.createModal.Visible || v.valueModal.Visible {
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
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutContent(ccgtx, th)
			}),
		)
	})
}

// layoutHeader renders the header section
func (v *CounterView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			title := material.Label(th.Material(), unit.Sp(24), "Distributed Counters")
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

// layoutActions renders the action buttons
func (v *CounterView) layoutActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	isSelected := v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered)

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			btn := components.Button(th, &v.createBtn, icons.ContentAddCircle, components.IconPositionStart, "Create")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			btn := components.SecondaryButton(th, &v.RefreshBtn, icons.NavigationRefresh, components.IconPositionStart, "Refresh")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.getBtn, icons.ActionVisibility, components.IconPositionStart, "Get Value")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.incrBtn, icons.ActionSwapHoriz, components.IconPositionStart, "Increment")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.decrBtn, icons.ActionSwapHoriz, components.IconPositionStart, "Decrement")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.DeleteBtn, icons.ActionDelete, components.IconPositionStart, "Delete")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Flexed(0.3, func(cgtx layout.Context) layout.Dimensions {
			return v.incrementInput.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.SearchEditor.Layout(cgtx, th)
		}),
	)
}

// layoutContent renders the main content area
func (v *CounterView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			return v.Split.Layout(cgtx, th,
				func(leftgtx layout.Context) layout.Dimensions {
					return v.layoutCountersTable(leftgtx, th)
				},
				func(rightgtx layout.Context) layout.Dimensions {
					return v.layoutCounterDetails(rightgtx, th)
				},
			)
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.createModal.Visible {
				return v.createModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.valueModal.Visible {
				return v.valueModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.ConfirmModal.IsVisible() {
				return v.ConfirmModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
	)
}

// layoutCountersTable renders the counters table
func (v *CounterView) layoutCountersTable(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.EmptyState {
		return v.layoutEmptyState(gtx, th)
	}

	startIdx := (v.Paginator.CurrentPage - 1) * v.PerPage
	endIdx := startIdx + v.PerPage
	if endIdx > len(v.filtered) {
		endIdx = len(v.filtered)
	}
	if startIdx < 0 || startIdx >= len(v.filtered) {
		startIdx = 0
		endIdx = 0
	}

	var pageCounters []*CounterInfo
	if endIdx > startIdx {
		pageCounters = v.filtered[startIdx:endIdx]
	}

	v.Table.Rows = make([]components.TableRow, len(pageCounters))
	for i, c := range pageCounters {
		v.Table.Rows[i] = components.TableRow{
			Values: []string{
				c.Name,
				fmt.Sprintf("%d", c.Value),
				c.Subject,
				c.Created,
			},
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

// layoutEmptyState renders the empty state
func (v *CounterView) layoutEmptyState(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return components.EmptyState{
		Icon:    icons.ActionLabel,
		Title:   "No Counters Found",
		Message: "Create a counter to get started with distributed counting.",
	}.Layout(gtx, th)
}

// layoutCounterDetails renders the details panel
func (v *CounterView) layoutCounterDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Select a counter to view details")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	counter := v.filtered[v.SelectedIdx]

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.Card{
					Title: counter.Name,
				}.Layout(ccgtx, th, func(cccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return components.StatCard{
								Title: "Current Value",
								Value: fmt.Sprintf("%d", counter.Value),
							}.Layout(c4gtx, th)
						}),
					)
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.Card{
					Title: "Details",
				}.Layout(ccgtx, th, func(cccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Name", counter.Name)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Subject", counter.Subject)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Created", counter.Created)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Updated", counter.Updated)
						}),
					)
				})
			}),
		)
	})
}

// promptCounterDelete shows a confirmation dialog for deleting a counter
func (v *CounterView) promptCounterDelete(name string) {
	v.ConfirmModal.Title = "Delete Counter"
	v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to delete counter '%s'?", name)
	v.ConfirmModal.ReturnFocus = v.Table.FocusTag()
	v.ConfirmModal.SetOnClose(func() {
		v.RestoreListFocus = true
	})
	v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
		if option == "Confirm" {
			v.deleteCounter(name)
		}
	})
	v.ConfirmModal.Show()
}

// isModalVisible returns true if any modal is visible
func (v *CounterView) isModalVisible() bool {
	return v.createModal.Visible || v.valueModal.Visible || v.ConfirmModal.IsVisible()
}

// handleTab handles TAB key navigation
func (v *CounterView) handleTab(gtx layout.Context, shift bool) {
	var tags []any
	if v.isModalVisible() {
		return
	}

	tags = []any{
		&v.createBtn,
		&v.RefreshBtn,
	}

	isSelected := v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered)
	if isSelected {
		tags = append(tags, &v.getBtn, &v.incrBtn, &v.decrBtn, &v.DeleteBtn)
	}

	tags = append(tags, v.incrementInput.FocusTag())
	tags = append(tags, v.SearchEditor.FocusTag())

	if !v.EmptyState && len(v.filtered) > 0 {
		tags = append(tags,
			v.Table.FocusTag(),
			v.Paginator.PrevClick,
			v.Paginator.NextClick,
		)
	}

	HandleTab(gtx, shift, tags, v.next, v.prev)
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *CounterView) HandleShortcuts(gtx layout.Context) bool {
	// TODO: Implement view-specific shortcuts
	return false
}

// GetShortcutsHelp returns help text for this view's shortcuts
func (v *CounterView) GetShortcutsHelp() []shortcuts.Shortcut {
	return nil
}
