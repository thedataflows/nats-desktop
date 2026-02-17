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

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"
	log "github.com/thedataflows/go-lib-log"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
)

type ConsumersView struct {
	*BaseView

	consumers []*ConsumerInfo
	filtered  []*ConsumerInfo

	// Extra buttons not in BaseView
	pauseBtn widget.Clickable
	resetBtn widget.Clickable
	editBtn  widget.Clickable
	copyBtn  widget.Clickable

	// Filter chips
	activeFilter *components.FilterChip
	pausedFilter *components.FilterChip
	idleFilter   *components.FilterChip
	// Track previous filter states to detect changes
	prevActiveFilter bool
	prevPausedFilter bool
	prevIdleFilter   bool

	// Create Consumer Modal
	createModal *components.FormModal

	// Edit Consumer Modal
	editModal *components.FormModal

	// Copy Consumer Modal
	copyModal     *components.FormModal
	copyNameInput *components.InputField

	// Mandatory fields
	streamInput       *components.InputField
	consumerNameInput *components.InputField

	// Optional fields (in expandable section)
	optionalSection         *components.ExpandableSection
	durableInput            *components.InputField
	descriptionInput        *components.InputField
	deliverPolicyInput      *components.DropDown
	ackPolicyInput          *components.DropDown
	maxDeliverInput         *components.InputField
	maxAckPendingInput      *components.InputField
	maxWaitingInput         *components.InputField
	maxRequestBatchInput    *components.InputField
	maxRequestExpiresInput  *components.InputField
	maxRequestMaxBytesInput *components.InputField
	replicasInput           *components.InputField
	inactiveThresholdInput  *components.InputField
	filterSubjectInput      *components.InputField

	next, prev any
}

type ConsumerInfo struct {
	Name         string
	Stream       string
	State        string
	Pending      int64
	AckPending   int64
	Delivered    int64
	AckFloor     string
	LastDelivery string
	Created      string
	Paused       bool
	NumDelivered uint64
	NumAcks      uint64
	Config       ConsumerConfigInfo
}

type ConsumerConfigInfo struct {
	Durable            string
	Description        string
	DeliverPolicy      string
	AckPolicy          string
	AckWait            string
	MaxDeliver         int
	MaxAckPending      int
	MaxWaiting         int
	MaxRequestBatch    int
	MaxRequestExpires  string
	MaxRequestMaxBytes int
	FilterSubject      string
	InactiveThreshold  string
	Replicas           int
}

func NewConsumersView(th *theme.Theme) *ConsumersView {
	v := &ConsumersView{
		BaseView: NewBaseView(
			[]string{"Name", "Stream", "State", "Pending", "Delivered", "Last Delivery", "Created"},
			10,
		),
		consumers:         []*ConsumerInfo{},
		filtered:          []*ConsumerInfo{},
		activeFilter:      components.NewFilterChip("Active"),
		pausedFilter:      components.NewFilterChip("Paused"),
		idleFilter:        components.NewFilterChip("Idle"),
		createModal:       components.NewFormModal("Create Consumer"),
		editModal:         components.NewFormModal("Edit Consumer"),
		copyModal:         components.NewFormModal("Copy Consumer"),
		streamInput:       components.NewLabeledInputFieldWithPosition("Stream", "", components.LabelPositionTop),
		consumerNameInput: components.NewLabeledInputFieldWithPosition("Name", "", components.LabelPositionTop),
		copyNameInput:     components.NewLabeledInputFieldWithPosition("New consumer name", "", components.LabelPositionTop),
		optionalSection:   components.NewExpandableSection("Advanced Options"),
		durableInput:      components.NewLabeledInputFieldWithPosition("Durable", "", components.LabelPositionTop),
		descriptionInput:  components.NewLabeledInputFieldWithPosition("Description", "", components.LabelPositionTop),
		deliverPolicyInput: components.NewLabeledDropDown("Deliver policy",
			components.NewDropDownOption("All").WithValue("all").DefaultSelected(),
			components.NewDropDownOption("New").WithValue("new"),
			components.NewDropDownOption("Last").WithValue("last"),
			components.NewDropDownOption("By Start Sequence").WithValue("by_start_sequence"),
		),
		ackPolicyInput: components.NewLabeledDropDown("Ack policy",
			components.NewDropDownOption("Explicit").WithValue("explicit").DefaultSelected(),
			components.NewDropDownOption("None").WithValue("none"),
			components.NewDropDownOption("All").WithValue("all"),
		),
		maxDeliverInput:         components.NewLabeledInputFieldWithPosition("Max deliver", "", components.LabelPositionTop),
		maxAckPendingInput:      components.NewLabeledInputFieldWithPosition("Max ack pending", "", components.LabelPositionTop),
		maxWaitingInput:         components.NewLabeledInputFieldWithPosition("Max waiting", "", components.LabelPositionTop),
		maxRequestBatchInput:    components.NewLabeledInputFieldWithPosition("Max batch", "", components.LabelPositionTop),
		maxRequestExpiresInput:  components.NewLabeledInputFieldWithPosition("Max expires", "", components.LabelPositionTop),
		maxRequestMaxBytesInput: components.NewLabeledInputFieldWithPosition("Max bytes", "", components.LabelPositionTop),
		replicasInput:           components.NewLabeledInputFieldWithPosition("Replicas", "", components.LabelPositionTop),
		inactiveThresholdInput:  components.NewLabeledInputFieldWithPosition("Inactive threshold", "", components.LabelPositionTop),
		filterSubjectInput:      components.NewLabeledInputFieldWithPosition("Filter subject", "", components.LabelPositionTop),
	}

	// All filters start unselected - show all consumers by default
	v.SearchEditor.Placeholder = "Search consumers..."
	v.SearchEditor.SetIcon(icons.ActionSearch, components.IconPositionStart)

	v.Split = components.SplitView{
		Resize: component.Resize{
			Ratio: 0.7,
		},
		BarWidth: unit.Dp(2),
	}

	// Set up create consumer modal with custom content
	v.createModal.ReturnFocus = v.Table.FocusTag()
	v.createModal.MaxWidth = unit.Dp(500)
	v.createModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.streamInput.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.consumerNameInput.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.optionalSection.Layout(cgtx, th, func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
						// Row 1: Durable | Description
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.durableInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.descriptionInput.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						// Row 2: Deliver Policy | Ack Policy
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.deliverPolicyInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.ackPolicyInput.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						// Row 3: Max Deliver | Max Ack Pending
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.maxDeliverInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.maxAckPendingInput.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						// Row 4: Max Waiting | Max Request Batch
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.maxWaitingInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.maxRequestBatchInput.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						// Row 5: Max Request Expires | Max Request Max Bytes
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.maxRequestExpiresInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.maxRequestMaxBytesInput.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						// Row 6: Replicas | Inactive Threshold
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.replicasInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.inactiveThresholdInput.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						// Row 7: Filter Subject (full width)
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.filterSubjectInput.Layout(cccgtx, th)
						}),
					)
				})
			}),
		)
	}
	// Set up TAB navigation order for the modal - using dynamic function to handle collapsed sections
	v.createModal.CustomFocusTagsFunc = func() []event.Tag {
		tags := []event.Tag{
			v.streamInput.FocusTag(),
			v.consumerNameInput.FocusTag(),
			v.optionalSection.FocusTag(),
		}
		// Only include optional section contents if expanded
		if v.optionalSection.Expanded {
			tags = append(tags,
				v.durableInput.FocusTag(),
				v.descriptionInput.FocusTag(),
				v.deliverPolicyInput.FocusTag(),
				v.ackPolicyInput.FocusTag(),
				v.maxDeliverInput.FocusTag(),
				v.maxAckPendingInput.FocusTag(),
				v.maxWaitingInput.FocusTag(),
				v.maxRequestBatchInput.FocusTag(),
				v.maxRequestExpiresInput.FocusTag(),
				v.maxRequestMaxBytesInput.FocusTag(),
				v.replicasInput.FocusTag(),
				v.inactiveThresholdInput.FocusTag(),
				v.filterSubjectInput.FocusTag(),
			)
		}
		return tags
	}
	v.createModal.OnSave = func() bool {
		return v.handleCreateConsumer()
	}
	v.createModal.OnClose = func() {
		v.RestoreListFocus = true
	}

	// Set up edit modal with same content as create
	v.editModal.ReturnFocus = v.Table.FocusTag()
	v.editModal.CustomContent = v.createModal.CustomContent
	v.editModal.CustomFocusTagsFunc = v.createModal.CustomFocusTagsFunc
	v.editModal.OnSave = func() bool {
		return v.handleEditConsumer()
	}
	v.editModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.editModal.MaxWidth = unit.Dp(500)

	// Set up copy modal
	v.copyModal.ReturnFocus = v.Table.FocusTag()
	v.copyModal.MaxWidth = unit.Dp(400)
	v.copyModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.copyNameInput.Layout(gtx, th)
	}
	v.copyModal.CustomFocusTags = []event.Tag{
		v.copyNameInput.FocusTag(),
	}
	v.copyModal.OnSave = func() bool {
		return v.handleCopyConsumer()
	}
	v.copyModal.OnClose = func() {
		v.RestoreListFocus = true
	}

	return v
}

func (v *ConsumersView) SetApp(app App) {
	v.App = app
	v.Table.OnCopyFeedback = func(text string) {
		if v.App != nil {
			v.App.ShowToast("Copied: "+text, components.ToastTypeSuccess)
		}
	}
}

func (v *ConsumersView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *ConsumersView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.ConsumersPageId,
		Title: "Consumers",
		Icon:  icons.ActionInput,
	}
}

func (v *ConsumersView) OnEnter() {
	v.Refresh()
}

func (v *ConsumersView) FirstFocusTag() any {
	return v.SearchEditor.FocusTag()
}

func (v *ConsumersView) LastFocusTag() any {
	return v.Paginator.NextClick
}

func (v *ConsumersView) showCreateConsumerModal() {
	// Clear all inputs
	v.streamInput.SetText("")
	v.consumerNameInput.SetText("")
	v.durableInput.SetText("")
	v.descriptionInput.SetText("")
	v.deliverPolicyInput.SetSelected(0)
	v.ackPolicyInput.SetSelected(0)
	v.maxDeliverInput.SetText("")
	v.maxAckPendingInput.SetText("")
	v.maxWaitingInput.SetText("")
	v.maxRequestBatchInput.SetText("")
	v.maxRequestExpiresInput.SetText("")
	v.maxRequestMaxBytesInput.SetText("")
	v.replicasInput.SetText("")
	v.inactiveThresholdInput.SetText("")
	v.filterSubjectInput.SetText("")
	v.optionalSection.Expanded = false

	v.createModal.Show()
}

func (v *ConsumersView) handleCreateConsumer() bool {
	stream := v.streamInput.GetText()
	name := v.consumerNameInput.GetText()

	if stream == "" || name == "" {
		if v.App != nil {
			v.App.ShowToast("Stream name and consumer name are required", components.ToastTypeError)
		}
		return false
	}

	// Validate numeric fields
	if maxDeliver := v.maxDeliverInput.GetText(); maxDeliver != "" {
		if _, err := strconv.Atoi(maxDeliver); err != nil {
			if v.App != nil {
				v.App.ShowToast("Max deliver must be a valid number", components.ToastTypeError)
			}
			return false
		}
	}

	if maxAckPending := v.maxAckPendingInput.GetText(); maxAckPending != "" {
		if val, err := strconv.Atoi(maxAckPending); err != nil || val < 0 {
			if v.App != nil {
				v.App.ShowToast("Max ack pending must be a valid non-negative number", components.ToastTypeError)
			}
			return false
		}
	}

	if maxWaiting := v.maxWaitingInput.GetText(); maxWaiting != "" {
		if val, err := strconv.Atoi(maxWaiting); err != nil || val < 0 {
			if v.App != nil {
				v.App.ShowToast("Max waiting must be a valid non-negative number", components.ToastTypeError)
			}
			return false
		}
	}

	if maxRequestBatch := v.maxRequestBatchInput.GetText(); maxRequestBatch != "" {
		if val, err := strconv.Atoi(maxRequestBatch); err != nil || val < 0 {
			if v.App != nil {
				v.App.ShowToast("Max request batch must be a valid non-negative number", components.ToastTypeError)
			}
			return false
		}
	}

	if maxRequestMaxBytes := v.maxRequestMaxBytesInput.GetText(); maxRequestMaxBytes != "" {
		if _, err := strconv.Atoi(maxRequestMaxBytes); err != nil {
			if v.App != nil {
				v.App.ShowToast("Max request max bytes must be a valid number", components.ToastTypeError)
			}
			return false
		}
	}

	if replicas := v.replicasInput.GetText(); replicas != "" {
		if val, err := strconv.Atoi(replicas); err != nil || val < 1 || val > 5 {
			if v.App != nil {
				v.App.ShowToast("Replicas must be a number between 1 and 5", components.ToastTypeError)
			}
			return false
		}
	}

	// Validate duration fields
	if maxRequestExpires := v.maxRequestExpiresInput.GetText(); maxRequestExpires != "" {
		if _, err := time.ParseDuration(maxRequestExpires); err != nil {
			if v.App != nil {
				v.App.ShowToast("Max request expires must be a valid duration (e.g., 30s, 5m, 1h)", components.ToastTypeError)
			}
			return false
		}
	}

	if inactiveThreshold := v.inactiveThresholdInput.GetText(); inactiveThreshold != "" {
		if _, err := time.ParseDuration(inactiveThreshold); err != nil {
			if v.App != nil {
				v.App.ShowToast("Inactive threshold must be a valid duration (e.g., 5m, 1h)", components.ToastTypeError)
			}
			return false
		}
	}

	v.createConsumerWithConfig(stream, name)
	return true
}

func (v *ConsumersView) createConsumerWithConfig(stream, name string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		config := jetstream.ConsumerConfig{
			Name: name,
		}

		// Parse optional fields
		durable := v.durableInput.GetText()
		if durable != "" {
			config.Durable = durable
		}

		desc := v.descriptionInput.GetText()
		if desc != "" {
			config.Description = desc
		}

		deliverPolicy := "all"
		if selected := v.deliverPolicyInput.GetSelected(); selected != nil {
			deliverPolicy = selected.Value
		}
		switch deliverPolicy {
		case "last":
			config.DeliverPolicy = jetstream.DeliverLastPolicy
		case "new":
			config.DeliverPolicy = jetstream.DeliverNewPolicy
		case "by_start_sequence":
			config.DeliverPolicy = jetstream.DeliverByStartSequencePolicy
		case "by_start_time":
			config.DeliverPolicy = jetstream.DeliverByStartTimePolicy
		default:
			config.DeliverPolicy = jetstream.DeliverAllPolicy
		}

		ackPolicy := "explicit"
		if selected := v.ackPolicyInput.GetSelected(); selected != nil {
			ackPolicy = selected.Value
		}
		switch ackPolicy {
		case "none":
			config.AckPolicy = jetstream.AckNonePolicy
		case "all":
			config.AckPolicy = jetstream.AckAllPolicy
		default:
			config.AckPolicy = jetstream.AckExplicitPolicy
		}

		if maxDeliver := v.maxDeliverInput.GetText(); maxDeliver != "" {
			if val, err := strconv.Atoi(maxDeliver); err == nil {
				config.MaxDeliver = val
			}
		}

		if maxAckPending := v.maxAckPendingInput.GetText(); maxAckPending != "" {
			if val, err := strconv.Atoi(maxAckPending); err == nil {
				config.MaxAckPending = val
			}
		}

		if maxWaiting := v.maxWaitingInput.GetText(); maxWaiting != "" {
			if val, err := strconv.Atoi(maxWaiting); err == nil {
				config.MaxWaiting = val
			}
		}

		if maxRequestBatch := v.maxRequestBatchInput.GetText(); maxRequestBatch != "" {
			if val, err := strconv.Atoi(maxRequestBatch); err == nil {
				config.MaxRequestBatch = val
			}
		}

		if maxRequestExpires := v.maxRequestExpiresInput.GetText(); maxRequestExpires != "" {
			if duration, err := time.ParseDuration(maxRequestExpires); err == nil {
				config.MaxRequestExpires = duration
			}
		}

		if maxRequestMaxBytes := v.maxRequestMaxBytesInput.GetText(); maxRequestMaxBytes != "" {
			if val, err := strconv.Atoi(maxRequestMaxBytes); err == nil {
				config.MaxRequestMaxBytes = val
			}
		}

		if replicas := v.replicasInput.GetText(); replicas != "" {
			if val, err := strconv.Atoi(replicas); err == nil {
				config.Replicas = val
			}
		}

		if inactiveThreshold := v.inactiveThresholdInput.GetText(); inactiveThreshold != "" {
			if duration, err := time.ParseDuration(inactiveThreshold); err == nil {
				config.InactiveThreshold = duration
			}
		}

		filterSubject := v.filterSubjectInput.GetText()
		if filterSubject != "" {
			config.FilterSubject = filterSubject
		}

		_, err := v.App.NATS().CreateConsumerWithConfig(ctx, stream, config)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to create consumer: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", stream).
				Str("consumer", name).
				Str("durable", config.Durable).
				Str("description", config.Description).
				Str("deliver_policy", deliverPolicy).
				Str("ack_policy", ackPolicy).
				Int("max_deliver", config.MaxDeliver).
				Int("max_ack_pending", config.MaxAckPending).
				Int("max_waiting", config.MaxWaiting).
				Int("replicas", config.Replicas).
				Str("filter_subject", config.FilterSubject).
				Err(err).
				Msg("Consumer creation failed")
		} else {
			v.App.ShowToast("Consumer created successfully", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("stream", stream).
				Str("consumer", name).
				Str("durable", config.Durable).
				Str("description", config.Description).
				Str("deliver_policy", deliverPolicy).
				Str("ack_policy", ackPolicy).
				Int("max_deliver", config.MaxDeliver).
				Int("max_ack_pending", config.MaxAckPending).
				Int("max_waiting", config.MaxWaiting).
				Int("replicas", config.Replicas).
				Str("filter_subject", config.FilterSubject).
				Msg("Consumer created")
			v.Refresh()
		}
	}()
}

func (v *ConsumersView) createConsumer(stream, name string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := v.App.NATS().CreateConsumer(ctx, stream, name)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to create consumer: %v", err), components.ToastTypeError)
		} else {
			v.App.ShowToast("Consumer created successfully", components.ToastTypeSuccess)
			v.Refresh()
		}
		if v.App.GetCurrentPageID() == navigator.ConsumersPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ConsumersView) pauseConsumer(stream, name string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := v.App.NATS().PauseConsumer(ctx, stream, name, 24*time.Hour)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to pause consumer: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", stream).
				Str("consumer", name).
				Dur("pause_duration", 24*time.Hour).
				Err(err).
				Msg("Consumer pause failed")
		} else {
			v.App.ShowToast("Consumer paused", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("stream", stream).
				Str("consumer", name).
				Dur("pause_duration", 24*time.Hour).
				Msg("Consumer paused")
			v.Refresh()
		}
		if v.App.GetCurrentPageID() == navigator.ConsumersPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ConsumersView) resumeConsumer(stream, name string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := v.App.NATS().ResumeConsumer(ctx, stream, name)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to resume consumer: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", stream).
				Str("consumer", name).
				Err(err).
				Msg("Consumer resume failed")
		} else {
			v.App.ShowToast("Consumer resumed", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("stream", stream).
				Str("consumer", name).
				Msg("Consumer resumed")
			v.Refresh()
		}
		if v.App.GetCurrentPageID() == navigator.ConsumersPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ConsumersView) deleteConsumer(stream, name string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := v.App.NATS().DeleteConsumer(ctx, stream, name)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to delete consumer: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", stream).
				Str("consumer", name).
				Err(err).
				Msg("Failed to delete consumer")
		} else {
			v.App.ShowToast("Consumer deleted", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("stream", stream).
				Str("consumer", name).
				Msg("Consumer deleted")
			v.SelectedIdx = -1
			v.Refresh()
		}
		if v.App.GetCurrentPageID() == navigator.ConsumersPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ConsumersView) Refresh() {
	if v.App == nil || v.Loading {
		return
	}

	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		v.consumers = []*ConsumerInfo{}
		v.EmptyState = true
		v.filterConsumers()
		return
	}

	v.Loading = true
	go func() {
		defer func() { v.Loading = false }()

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		streams, err := client.ListStreams(ctx)
		if err != nil {
			v.App.ShowToast("Failed to list streams: "+err.Error(), components.ToastTypeError)
			return
		}

		newConsumers := make([]*ConsumerInfo, 0)
		for _, s := range streams {
			stream, err := client.GetJetStream().Stream(ctx, s.Config.Name)
			if err != nil {
				v.App.ShowToast("Error getting stream "+s.Config.Name+": "+err.Error(), components.ToastTypeWarning)
				continue
			}
			iterator := stream.ListConsumers(ctx)
			for info := range iterator.Info() {
				if info != nil {
					state := "Active"
					if info.Paused {
						state = "Paused"
					} else if info.NumPending == 0 && info.NumAckPending == 0 {
						state = "Idle"
					}

					configInfo := ConsumerConfigInfo{
						Durable:            info.Config.Durable,
						Description:        info.Config.Description,
						DeliverPolicy:      info.Config.DeliverPolicy.String(),
						AckPolicy:          info.Config.AckPolicy.String(),
						AckWait:            info.Config.AckWait.String(),
						MaxDeliver:         info.Config.MaxDeliver,
						MaxAckPending:      info.Config.MaxAckPending,
						MaxWaiting:         info.Config.MaxWaiting,
						MaxRequestBatch:    info.Config.MaxRequestBatch,
						MaxRequestExpires:  info.Config.MaxRequestExpires.String(),
						MaxRequestMaxBytes: info.Config.MaxRequestMaxBytes,
						FilterSubject:      info.Config.FilterSubject,
						InactiveThreshold:  info.Config.InactiveThreshold.String(),
						Replicas:           info.Config.Replicas,
					}
					if configInfo.DeliverPolicy == "" {
						configInfo.DeliverPolicy = "all"
					}
					if configInfo.AckPolicy == "" {
						configInfo.AckPolicy = "explicit"
					}
					newConsumers = append(newConsumers, &ConsumerInfo{
						Name:         info.Name,
						Stream:       s.Config.Name,
						State:        state,
						Pending:      int64(info.NumPending),
						AckPending:   int64(info.NumAckPending),
						Delivered:    int64(info.Delivered.Consumer),
						AckFloor:     fmt.Sprintf("%d", info.AckFloor.Consumer),
						LastDelivery: "",
						Created:      info.Created.Format("2006-01-02 15:04:05"),
						Paused:       info.Paused,
						NumDelivered: info.Delivered.Consumer,
						Config:       configInfo,
					})
				}
			}
			if iterator.Err() != nil {
				v.App.ShowToast("Error iterating consumers for "+s.Config.Name+": "+iterator.Err().Error(), components.ToastTypeWarning)
			}
		}

		v.consumers = newConsumers
		v.EmptyState = len(newConsumers) == 0
		v.filterConsumers()
		if v.App != nil && v.App.GetCurrentPageID() == navigator.ConsumersPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ConsumersView) filterConsumers() {
	query := strings.ToLower(v.SearchEditor.GetText())
	v.filtered = make([]*ConsumerInfo, 0)

	for _, c := range v.consumers {
		// Check search query
		if query != "" &&
			!strings.Contains(strings.ToLower(c.Name), query) &&
			!strings.Contains(strings.ToLower(c.Stream), query) {
			continue
		}

		// Check state filters - include if no filters selected OR if state matches a selected filter
		if !v.activeFilter.Selected && !v.pausedFilter.Selected && !v.idleFilter.Selected {
			// No filters selected, show all
			v.filtered = append(v.filtered, c)
		} else if v.activeFilter.Selected && c.State == "Active" {
			v.filtered = append(v.filtered, c)
		} else if v.pausedFilter.Selected && c.State == "Paused" {
			v.filtered = append(v.filtered, c)
		} else if v.idleFilter.Selected && c.State == "Idle" {
			v.filtered = append(v.filtered, c)
		}
		// If none of the above, consumer is filtered out
	}

	totalPages := (len(v.filtered) + v.PerPage - 1) / v.PerPage
	v.Paginator.SetTotalPages(totalPages)
	v.Table.ResetWidths()

	// Trigger UI refresh after filtering
	if v.App != nil {
		v.App.Invalidate()
	}
}

func (v *ConsumersView) addSampleData() {
	v.consumers = []*ConsumerInfo{
		{
			Name:         "ORDERS-worker-1",
			Stream:       "ORDERS",
			State:        "Active",
			Pending:      123,
			AckPending:   45,
			Delivered:    45234,
			AckFloor:     "45234",
			LastDelivery: "2s ago",
			Created:      "2024-01-20",
			NumDelivered: 45234,
			Config: ConsumerConfigInfo{
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				MaxDeliver:    -1,
				MaxAckPending: 1000,
				FilterSubject: "orders.>",
			},
		},
		{
			Name:         "ORDERS-worker-2",
			Stream:       "ORDERS",
			State:        "Active",
			Pending:      89,
			AckPending:   32,
			Delivered:    31209,
			AckFloor:     "31209",
			LastDelivery: "1s ago",
			Created:      "2024-01-20",
			NumDelivered: 31209,
			Config: ConsumerConfigInfo{
				DeliverPolicy: "last",
				AckPolicy:     "explicit",
				MaxDeliver:    3,
				MaxAckPending: 500,
				FilterSubject: "orders.processed",
			},
		},
		{
			Name:         "EVENTS-subscriber",
			Stream:       "EVENTS",
			State:        "Idle",
			Pending:      0,
			AckPending:   0,
			Delivered:    15234,
			AckFloor:     "15234",
			LastDelivery: "5m ago",
			Created:      "2024-01-22",
			NumDelivered: 15234,
			Config: ConsumerConfigInfo{
				DeliverPolicy: "new",
				AckPolicy:     "none",
				MaxDeliver:    -1,
				MaxAckPending: 1000,
				FilterSubject: "",
			},
		},
		{
			Name:         "METRICS-processor",
			Stream:       "METRICS",
			State:        "Paused",
			Pending:      567,
			AckPending:   234,
			Delivered:    890123,
			AckFloor:     "890123",
			LastDelivery: "10s ago",
			Created:      "2024-01-10",
			Paused:       true,
			NumDelivered: 890123,
			Config: ConsumerConfigInfo{
				DeliverPolicy: "all",
				AckPolicy:     "all",
				MaxDeliver:    5,
				MaxAckPending: 2000,
				FilterSubject: "metrics.*",
			},
		},
	}
	v.EmptyState = false
}

func (v *ConsumersView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.RestoreListFocus {
		v.RestoreListFocus = false
		gtx.Execute(key.FocusCmd{Tag: v.Table.FocusTag()})
	}

	// Only handle tab navigation if no modal is visible
	// The modals handle their own TAB navigation internally
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

		// Handle Enter key to open edit modal
		for {
			ev, ok := gtx.Event(key.Filter{Name: key.NameEnter}, key.Filter{Name: key.NameReturn})
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
					v.showEditConsumerModal()
				}
			}
		}
	}

	for v.AddBtn.Clicked(gtx) {
		v.showCreateConsumerModal()
	}

	if v.SearchEditor.Changed() {
		v.LastFilterTime = gtx.Now
	}

	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
			v.filterConsumers()
			v.LastFilterTime = time.Time{}
		} else {
			gtx.Execute(op.InvalidateCmd{At: v.LastFilterTime.Add(300 * time.Millisecond)})
		}
	}

	for v.RefreshBtn.Clicked(gtx) {
		v.Refresh()
	}

	for v.pauseBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			consumer := v.filtered[v.SelectedIdx]
			if consumer.State == "Paused" {
				v.resumeConsumer(consumer.Stream, consumer.Name)
			} else {
				v.pauseConsumer(consumer.Stream, consumer.Name)
			}
		}
	}

	for v.resetBtn.Clicked(gtx) {
		if v.App != nil {
			v.App.ShowToast("Reset consumer coming soon", components.ToastTypeInfo)
		}
	}

	for v.editBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.showEditConsumerModal()
		}
	}

	for v.copyBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.copyNameInput.SetText("")
			v.copyModal.Show()
		}
	}

	for v.DeleteBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			consumer := v.filtered[v.SelectedIdx]
			v.ConfirmModal.Title = "Delete Consumer"
			v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to delete consumer '%s' from stream '%s'?", consumer.Name, consumer.Stream)
			v.ConfirmModal.ReturnFocus = v.Table.FocusTag()
			v.ConfirmModal.SetOnClose(func() {
				v.RestoreListFocus = true
			})
			v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
				if option == "Confirm" {
					v.deleteConsumer(consumer.Stream, consumer.Name)
				}
			})
			v.ConfirmModal.Show()
		}
	}

	if v.Paginator.Next(gtx) {
		v.Paginator.NextPage()
	}

	if v.Paginator.Prev(gtx) {
		v.Paginator.PrevPage()
	}

	clicked := v.Table.Clicked()
	doubleClicked := v.Table.DoubleClicked()
	if clicked || doubleClicked {
		newIdx := (v.Paginator.CurrentPage-1)*v.PerPage + v.Table.SelectedRow
		if doubleClicked {
			if newIdx >= 0 && newIdx < len(v.filtered) {
				v.SelectedIdx = newIdx
				v.showEditConsumerModal()
			}
		}
		v.SelectedIdx = newIdx
	}

	if v.Table.SelectionChanged() {
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
			if v.createModal.Visible {
				return v.createModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.editModal.Visible {
				return v.editModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.copyModal.Visible {
				return v.copyModal.Layout(cgtx, th)
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

func (v *ConsumersView) isModalVisible() bool {
	return v.createModal.Visible || v.editModal.Visible || v.copyModal.Visible || v.ConfirmModal.IsVisible()
}

func (v *ConsumersView) handleTab(gtx layout.Context, shift bool) {
	var tags []any
	if v.createModal.Visible || v.editModal.Visible || v.copyModal.Visible {
		return
	} else {
		tags = []any{
			&v.AddBtn,
			&v.RefreshBtn,
		}

		isSelected := v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered)
		if isSelected {
			tags = append(tags, &v.pauseBtn, &v.resetBtn, &v.editBtn, &v.copyBtn, &v.DeleteBtn)
		}

		tags = append(tags,
			v.SearchEditor.FocusTag(),
			v.activeFilter.FocusTag(),
			v.pausedFilter.FocusTag(),
			v.idleFilter.FocusTag(),
		)

		if !v.EmptyState && len(v.filtered) > 0 {
			tags = append(tags,
				v.Table.FocusTag(),
				v.Paginator.PrevClick,
				v.Paginator.NextClick,
			)
		}
	}

	HandleTab(gtx, shift, tags, v.next, v.prev)
}

func (v *ConsumersView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(24), "JetStream Consumers")
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

func (v *ConsumersView) layoutActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	isSelected := v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			btn := components.Button(th, &v.AddBtn, icons.ContentAddCircle, components.IconPositionStart, "Create")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			btn := components.SecondaryButton(th, &v.RefreshBtn, icons.NavigationRefresh, components.IconPositionStart, "Refresh")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			pauseText := "Pause"
			if isSelected {
				if v.filtered[v.SelectedIdx].State == "Paused" {
					pauseText = "Resume"
				}
			} else {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.pauseBtn, icons.AVPause, components.IconPositionStart, pauseText)
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.resetBtn, icons.NavigationRefresh, components.IconPositionStart, "Reset")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.editBtn, icons.EditorModeEdit, components.IconPositionStart, "Edit")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.copyBtn, icons.ContentContentCopy, components.IconPositionStart, "Copy")
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
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.SearchEditor.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.activeFilter.Layout(cgtx, th)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.pausedFilter.Layout(cgtx, th)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.idleFilter.Layout(cgtx, th)
		}),
	)
}

func (v *ConsumersView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.EmptyState {
		return v.layoutEmptyState(gtx, th)
	}

	return v.Split.Layout(gtx, th,
		func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
					return v.layoutConsumersTable(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return v.Paginator.Layout(ccgtx, th)
				}),
			)
		},
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutConsumerDetails(cgtx, th)
		},
	)
}

func (v *ConsumersView) layoutEmptyState(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return components.EmptyState{
		Icon:    icons.ActionInput,
		Title:   "No Consumers Found",
		Message: "Create a JetStream consumer to get started.",
	}.Layout(gtx, th)
}

func (v *ConsumersView) layoutConsumersTable(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.Table.Rows = BuildTableRows(v.filtered, v.Paginator.CurrentPage, v.PerPage,
		func(c *ConsumerInfo, idx int) components.TableRow {
			return components.TableRow{
				Values: []string{
					c.Name,
					c.Stream,
					c.State,
					fmt.Sprintf("%d", c.Pending),
					fmt.Sprintf("%d", c.Delivered),
					c.LastDelivery,
					c.Created,
				},
			}
		}, v.SelectedIdx)

	return v.Table.Layout(gtx, th)
}

func (v *ConsumersView) layoutConsumerDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Select a consumer")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	consumer := v.filtered[v.SelectedIdx]

	// Calculate max value for bar chart scaling
	maxValue := consumer.Pending
	if consumer.AckPending > maxValue {
		maxValue = consumer.AckPending
	}
	if consumer.Delivered > maxValue {
		maxValue = consumer.Delivered
	}
	if maxValue == 0 {
		maxValue = 1
	}

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			// State pill
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				stateType := components.StatusPillNeutral
				var icon *widget.Icon
				switch consumer.State {
				case "Active":
					stateType = components.StatusPillSuccess
				case "Paused":
					stateType = components.StatusPillWarning
					icon = icons.AVPause
				}
				return components.StatusPill{
					Text: consumer.State,
					Type: stateType,
					Icon: icon,
				}.Layout(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			// Quick stats row
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Spacing: layout.SpaceBetween}.Layout(ccgtx,
					layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
						return components.StatCard{
							Title: "Pending",
							Value: fmt.Sprintf("%d", consumer.Pending),
						}.Layout(c5gtx, th)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
						return components.StatCard{
							Title: "Delivered",
							Value: fmt.Sprintf("%d", consumer.Delivered),
						}.Layout(c5gtx, th)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
						return components.StatCard{
							Title: "Ack Pending",
							Value: fmt.Sprintf("%d", consumer.AckPending),
						}.Layout(c5gtx, th)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			// Bar chart
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.Card{Title: "Statistics"}.Layout(ccgtx, th, func(c3gtx layout.Context) layout.Dimensions {
					barHeight := c3gtx.Dp(unit.Dp(16))
					maxBarWidth := c3gtx.Constraints.Max.X - c3gtx.Dp(unit.Dp(100))

					return layout.Flex{Axis: layout.Vertical}.Layout(c3gtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return v.layoutStatBar(c4gtx, th, "Pending", consumer.Pending, maxValue, maxBarWidth, barHeight, color.NRGBA{R: 255, G: 152, B: 0, A: 255})
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return v.layoutStatBar(c4gtx, th, "Ack Pending", consumer.AckPending, maxValue, maxBarWidth, barHeight, color.NRGBA{R: 76, G: 175, B: 80, A: 255})
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return v.layoutStatBar(c4gtx, th, "Delivered", consumer.Delivered, maxValue, maxBarWidth, barHeight, color.NRGBA{R: 69, G: 137, B: 245, A: 255})
						}),
					)
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			// Details card
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.Card{Title: "Details"}.Layout(ccgtx, th, func(c3gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(c3gtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Stream", consumer.Stream)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Ack Floor", consumer.AckFloor)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Total Delivered", fmt.Sprintf("%d", consumer.NumDelivered))
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Last Delivery", consumer.LastDelivery)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Created", consumer.Created)
						}),
					)
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			// Configuration card
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.Card{Title: "Configuration"}.Layout(ccgtx, th, func(c3gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(c3gtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Deliver Policy", consumer.Config.DeliverPolicy)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Ack Policy", consumer.Config.AckPolicy)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Max Deliver", fmt.Sprintf("%d", consumer.Config.MaxDeliver))
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Max Ack Pending", fmt.Sprintf("%d", consumer.Config.MaxAckPending))
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							filter := consumer.Config.FilterSubject
							if filter == "" {
								filter = "(all subjects)"
							}
							return layoutDetailRow(c4gtx, th, "Filter Subject", filter)
						}),
					)
				})
			}),
		)
	})
}

// showEditConsumerModal shows the edit modal with current consumer values
func (v *ConsumersView) showEditConsumerModal() {
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return
	}
	consumer := v.filtered[v.SelectedIdx]
	config := consumer.Config

	// Pre-fill mandatory fields
	v.streamInput.SetText(consumer.Stream)
	v.consumerNameInput.SetText(consumer.Name)

	// Pre-fill optional fields
	v.durableInput.SetText(config.Durable)
	v.descriptionInput.SetText(config.Description)
	v.deliverPolicyInput.SetSelectedByValue(config.DeliverPolicy)
	v.ackPolicyInput.SetSelectedByValue(config.AckPolicy)
	v.maxDeliverInput.SetText(strconv.Itoa(config.MaxDeliver))
	v.maxAckPendingInput.SetText(strconv.Itoa(config.MaxAckPending))
	v.maxWaitingInput.SetText(strconv.Itoa(config.MaxWaiting))
	v.maxRequestBatchInput.SetText(strconv.Itoa(config.MaxRequestBatch))
	v.maxRequestExpiresInput.SetText(config.MaxRequestExpires)
	v.maxRequestMaxBytesInput.SetText(strconv.Itoa(config.MaxRequestMaxBytes))
	v.filterSubjectInput.SetText(config.FilterSubject)
	v.inactiveThresholdInput.SetText(config.InactiveThreshold)
	v.replicasInput.SetText(strconv.Itoa(config.Replicas))

	// Expand the optional section to show fields
	v.optionalSection.Expanded = true

	v.editModal.Show()
}

// handleEditConsumer handles the edit consumer action
func (v *ConsumersView) handleEditConsumer() bool {
	stream := v.streamInput.GetText()
	name := v.consumerNameInput.GetText()
	if stream == "" || name == "" {
		if v.App != nil {
			v.App.ShowToast("Stream name and consumer name are required", components.ToastTypeError)
		}
		return false
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		config := jetstream.ConsumerConfig{Name: name}

		// Parse all optional fields
		if durable := v.durableInput.GetText(); durable != "" {
			config.Durable = durable
		}
		if desc := v.descriptionInput.GetText(); desc != "" {
			config.Description = desc
		}
		if selected := v.deliverPolicyInput.GetSelected(); selected != nil {
			switch selected.Value {
			case "all":
				config.DeliverPolicy = jetstream.DeliverAllPolicy
			case "last":
				config.DeliverPolicy = jetstream.DeliverLastPolicy
			case "new":
				config.DeliverPolicy = jetstream.DeliverNewPolicy
			}
		}
		if selected := v.ackPolicyInput.GetSelected(); selected != nil {
			switch selected.Value {
			case "none":
				config.AckPolicy = jetstream.AckNonePolicy
			case "all":
				config.AckPolicy = jetstream.AckAllPolicy
			case "explicit":
				config.AckPolicy = jetstream.AckExplicitPolicy
			}
		}
		if maxDeliver := v.maxDeliverInput.GetText(); maxDeliver != "" {
			if val, err := strconv.Atoi(maxDeliver); err == nil {
				config.MaxDeliver = val
			}
		}
		if maxAckPending := v.maxAckPendingInput.GetText(); maxAckPending != "" {
			if val, err := strconv.Atoi(maxAckPending); err == nil {
				config.MaxAckPending = val
			}
		}
		if maxWaiting := v.maxWaitingInput.GetText(); maxWaiting != "" {
			if val, err := strconv.Atoi(maxWaiting); err == nil {
				config.MaxWaiting = val
			}
		}
		if maxBatch := v.maxRequestBatchInput.GetText(); maxBatch != "" {
			if val, err := strconv.Atoi(maxBatch); err == nil {
				config.MaxRequestBatch = val
			}
		}
		if maxExpires := v.maxRequestExpiresInput.GetText(); maxExpires != "" {
			if duration, err := time.ParseDuration(maxExpires); err == nil {
				config.MaxRequestExpires = duration
			}
		}
		if maxBytes := v.maxRequestMaxBytesInput.GetText(); maxBytes != "" {
			if val, err := strconv.Atoi(maxBytes); err == nil {
				config.MaxRequestMaxBytes = val
			}
		}
		if filterSubject := v.filterSubjectInput.GetText(); filterSubject != "" {
			config.FilterSubject = filterSubject
		}
		if inactiveThreshold := v.inactiveThresholdInput.GetText(); inactiveThreshold != "" {
			if duration, err := time.ParseDuration(inactiveThreshold); err == nil {
				config.InactiveThreshold = duration
			}
		}
		if replicas := v.replicasInput.GetText(); replicas != "" {
			if val, err := strconv.Atoi(replicas); err == nil {
				config.Replicas = val
			}
		}

		client := v.App.GetNatsClient()
		_, err := client.UpdateConsumer(ctx, stream, config)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to update consumer: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", stream).
				Str("consumer", name).
				Str("durable", config.Durable).
				Str("description", config.Description).
				Int("max_deliver", config.MaxDeliver).
				Int("max_ack_pending", config.MaxAckPending).
				Int("max_waiting", config.MaxWaiting).
				Int("replicas", config.Replicas).
				Str("filter_subject", config.FilterSubject).
				Err(err).
				Msg("Consumer update failed")
		} else {
			v.App.ShowToast("Consumer updated successfully", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("stream", stream).
				Str("consumer", name).
				Str("durable", config.Durable).
				Str("description", config.Description).
				Int("max_deliver", config.MaxDeliver).
				Int("max_ack_pending", config.MaxAckPending).
				Int("max_waiting", config.MaxWaiting).
				Int("replicas", config.Replicas).
				Str("filter_subject", config.FilterSubject).
				Msg("Consumer updated")
			v.Refresh()
		}
	}()
	return true
}

// handleCopyConsumer handles the copy consumer action
func (v *ConsumersView) handleCopyConsumer() bool {
	newName := v.copyNameInput.GetText()
	if newName == "" {
		if v.App != nil {
			v.App.ShowToast("Please enter a new consumer name", components.ToastTypeError)
		}
		return false
	}
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return false
	}
	sourceConsumer := v.filtered[v.SelectedIdx]
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client := v.App.GetNatsClient()
		_, err := client.CopyConsumer(ctx, sourceConsumer.Stream, sourceConsumer.Name, newName)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to copy consumer: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", sourceConsumer.Stream).
				Str("source_consumer", sourceConsumer.Name).
				Str("new_consumer", newName).
				Err(err).
				Msg("Consumer copy failed")
			return
		}
		v.App.ShowToast(fmt.Sprintf("Consumer copied: %s -> %s", sourceConsumer.Name, newName), components.ToastTypeSuccess)
		log.Logger().Info().
			Str("stream", sourceConsumer.Stream).
			Str("source_consumer", sourceConsumer.Name).
			Str("new_consumer", newName).
			Msg("Consumer copied")
		v.Refresh()
	}()
	return true
}

// layoutStatBar renders a single horizontal bar for statistics
func (v *ConsumersView) layoutStatBar(gtx layout.Context, th *theme.Theme, label string, value int64, maxValue int64, maxWidth int, barHeight int, barColor color.NRGBA) layout.Dimensions {
	// Calculate bar width based on value
	barWidth := int(float64(value) / float64(maxValue) * float64(maxWidth))
	if barWidth < 4 && value > 0 {
		barWidth = 4 // Minimum visible width
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		// Label
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(8)}.Layout(cgtx, func(c2gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(12), label)
				lbl.Color = th.TextColor
				return lbl.Layout(c2gtx)
			})
		}),
		// Bar
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			dims := layout.Dimensions{Size: image.Pt(barWidth, barHeight)}
			clipStack := clip.Rect{Max: dims.Size}.Push(cgtx.Ops)
			paint.FillShape(cgtx.Ops, barColor, clip.Rect{
				Min: image.Pt(0, 0),
				Max: image.Pt(barWidth, barHeight),
			}.Op())
			clipStack.Pop()
			return dims
		}),
		// Value label
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8)}.Layout(cgtx, func(c2gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(12), fmt.Sprintf("%d", value))
				lbl.Color = th.TextColor
				return lbl.Layout(c2gtx)
			})
		}),
	)
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *ConsumersView) HandleShortcuts(gtx layout.Context) bool {
	// Check if any modal is visible first - view shortcuts disabled when modal is open
	if v.isModalVisible() {
		return false
	}

	// View-specific shortcuts - process only ONE event per frame
	ev, ok := gtx.Event(
		key.Filter{Name: key.Name("N"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("R"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("E"), Optional: key.ModShortcut},
		key.Filter{Name: key.NameDeleteForward},
		key.Filter{Name: key.NameDeleteBackward},
		key.Filter{Name: key.NameReturn},
		key.Filter{Name: key.NameEnter},
	)
	if !ok {
		return false
	}

	if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
		switch {
		case ke.Name == key.Name("N") && ke.Modifiers.Contain(key.ModShortcut):
			v.AddBtn.Click()
			return true
		case ke.Name == key.Name("R") && ke.Modifiers.Contain(key.ModShortcut):
			v.RefreshBtn.Click()
			return true
		case ke.Name == key.Name("E") && ke.Modifiers.Contain(key.ModShortcut):
			if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
				v.editBtn.Click()
				return true
			}
		case ke.Name == key.NameDeleteForward || ke.Name == key.NameDeleteBackward:
			if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
				v.DeleteBtn.Click()
				return true
			}
		case ke.Name == key.NameReturn || ke.Name == key.NameEnter:
			if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
				v.editBtn.Click()
				return true
			}
		}
	}
	return false
}

// GetShortcutsHelp returns help text for this view's shortcuts
func (v *ConsumersView) GetShortcutsHelp() []shortcuts.Shortcut {
	return []shortcuts.Shortcut{
		shortcuts.Create(func() {}),
		shortcuts.Refresh(func() {}),
		shortcuts.Delete(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.Edit(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.CopyName(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.CopyRow(func() bool { return v.SelectedIdx >= 0 }, func() {}),
	}
}
