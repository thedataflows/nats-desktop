package views

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sqweek/dialog"
	log "github.com/thedataflows/go-lib-log"
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

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
)

type JetStreamSubscriptionConfig struct {
	Subject       string
	Durable       string
	DeliverPolicy string
	AckPolicy     string
	HeadersOnly   bool
	MaxDeliver    int
	StartSeq      uint64
}

type PubSubView struct {
	*BaseView

	// Extra buttons not in BaseView
	pubBtn     widget.Clickable
	subBtn     widget.Clickable
	requestBtn widget.Clickable
	replyBtn   widget.Clickable
	clearBtn   widget.Clickable
	exportBtn  widget.Clickable

	// JetStream subscribe button
	jsSubBtn widget.Clickable

	// Request configuration
	requestModal        *components.FormModal
	requestTimeoutInput *components.InputField
	requestMaxReplies   *components.InputField

	subjectEditor *components.InputField
	queueEditor   *components.InputField
	codeEditor    *components.CodeEditor
	payloadEditor *components.CodeEditor

	// Advanced publish fields
	countInput      *components.InputField
	headersEditor   *components.CodeEditor
	translateEditor *components.InputField
	useTemplates    bool
	useHeaders      bool
	useTranslate    bool
	useAck          bool

	// Reply service fields
	replySubjectEditor  *components.InputField
	replyPayloadEditor  *components.CodeEditor
	replyCommandEditor  *components.InputField
	replyTypeSelected   int // 0 = static, 1 = template, 2 = command
	replyActiveServices map[string]*nats.Subscription
	replyServiceList    []string

	messagesSent   int
	messagesRecv   int
	messageHistory []*MessageEntry
	filtered       []*MessageEntry

	allFilter  *components.FilterChip
	sentFilter *components.FilterChip
	recvFilter *components.FilterChip

	mainSplit components.SplitView

	sub *nats.Subscription
	mu  sync.Mutex

	next, prev any

	// JetStream subscription fields
	showJSSubModal        bool
	jsSubCloseBtn         widget.Clickable
	jsSubSaveBtn          widget.Clickable
	durableNameInput      *components.InputField
	deliverPolicyDropDown *components.DropDown
	ackPolicyDropDown     *components.DropDown
	maxDeliverInput       *components.InputField
	headersOnlyBool       widget.Bool
	headersOnlyCheck      components.CheckBoxStyle
	startSeqInput         *components.InputField
	currentJSConsCtx      jetstream.ConsumeContext
	currentJSConsumer     string
	currentJSStream       string

	// Export modal fields
	exportModal        *components.FormModal
	exportFormatSelect *components.DropDown
	exportFilterInput  *components.InputField
}

type MessageEntry struct {
	Subject string
	Type    string
	Time    string
	Size    string
	Status  string
	Payload string
}

func NewPubSubView(th *theme.Theme) *PubSubView {
	v := &PubSubView{
		BaseView: NewBaseView(
			[]string{"Subject", "Type", "Time", "Size", "Status"},
			10,
		),
		messagesSent:        0,
		messagesRecv:        0,
		allFilter:           components.NewFilterChip("All"),
		sentFilter:          components.NewFilterChip("Published"),
		recvFilter:          components.NewFilterChip("Received"),
		messageHistory:      []*MessageEntry{},
		filtered:            []*MessageEntry{},
		replyActiveServices: make(map[string]*nats.Subscription),
		replyServiceList:    []string{},
	}
	v.subjectEditor = components.NewLabeledInputFieldWithPosition("Subject", "Enter subject...", components.LabelPositionTop)
	v.subjectEditor.SetIcon(icons.ActionLabel, components.IconPositionStart)
	v.queueEditor = components.NewLabeledInputFieldWithPosition("Queue group", "Optional", components.LabelPositionTop)
	v.queueEditor.SetIcon(icons.ActionSettings, components.IconPositionStart)
	v.codeEditor = components.NewCodeEditor("", components.CodeLanguageJSON, th)
	v.codeEditor.SetReadOnly(false)

	v.payloadEditor = components.NewCodeEditor("", components.CodeLanguageJSON, th)
	v.payloadEditor.SetReadOnly(true)

	// Initialize advanced publish fields
	v.countInput = components.NewLabeledInputFieldWithPosition("Count", "1", components.LabelPositionTop)
	v.countInput.SetIcon(icons.ContentFilterList, components.IconPositionStart)
	v.headersEditor = components.NewCodeEditor("", components.CodeLanguageJSON, th)
	v.headersEditor.SetReadOnly(false)
	v.translateEditor = components.NewLabeledInputFieldWithPosition("Translate", "jq .", components.LabelPositionTop)
	v.translateEditor.SetIcon(icons.ActionSettings, components.IconPositionStart)

	// Initialize reply service editors
	v.replySubjectEditor = components.NewLabeledInputFieldWithPosition("Subject", "e.g., service.>", components.LabelPositionTop)
	v.replySubjectEditor.SetIcon(icons.ActionLabel, components.IconPositionStart)
	v.replyPayloadEditor = components.NewCodeEditor("", components.CodeLanguageJSON, th)
	v.replyPayloadEditor.SetReadOnly(false)
	v.replyCommandEditor = components.NewLabeledInputFieldWithPosition("Command", "Command to execute...", components.LabelPositionTop)
	v.replyCommandEditor.SetIcon(icons.ActionSettings, components.IconPositionStart)

	// Initialize JetStream subscription fields
	v.durableNameInput = components.NewLabeledInputFieldWithPosition("Durable name", "e.g., my-durable-consumer", components.LabelPositionTop)
	v.deliverPolicyDropDown = components.NewDropDown(
		components.NewDropDownOption("All").WithValue("all"),
		components.NewDropDownOption("New").WithValue("new"),
		components.NewDropDownOption("Last").WithValue("last"),
		components.NewDropDownOption("By Start Sequence").WithValue("by_start_sequence"),
	)
	v.ackPolicyDropDown = components.NewDropDown(
		components.NewDropDownOption("None").WithValue("none"),
		components.NewDropDownOption("Explicit").WithValue("explicit"),
		components.NewDropDownOption("All").WithValue("all"),
	)
	v.maxDeliverInput = components.NewLabeledInputFieldWithPosition("Max deliver", "-1 for unlimited", components.LabelPositionTop)
	v.startSeqInput = components.NewLabeledInputFieldWithPosition("Start sequence", "0", components.LabelPositionTop)
	// Initialize checkbox with theme for focus support
	v.headersOnlyCheck = components.CheckBox(th.Material(), &v.headersOnlyBool, "Headers Only")
	v.headersOnlyCheck.SetTheme(th)

	// Initialize request modal
	v.requestModal = components.NewFormModal("Request Options")
	v.requestModal.MaxWidth = unit.Dp(400)
	v.requestTimeoutInput = components.NewLabeledInputFieldWithPosition("Timeout", "e.g., 5s, 1m", components.LabelPositionTop)
	v.requestTimeoutInput.SetIcon(icons.ActionSettings, components.IconPositionStart)
	v.requestMaxReplies = components.NewLabeledInputFieldWithPosition("Max Replies", "1 for single reply", components.LabelPositionTop)
	v.requestMaxReplies.SetIcon(icons.ContentFilterList, components.IconPositionStart)
	v.requestModal.ReturnFocus = &v.requestBtn
	v.requestModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.requestTimeoutInput.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.requestMaxReplies.Layout(cgtx, th)
			}),
		)
	}
	v.requestModal.CustomFocusTags = []event.Tag{
		v.requestTimeoutInput.FocusTag(),
		v.requestMaxReplies.FocusTag(),
	}
	v.requestModal.OnSave = func() bool {
		return v.handleRequestWithOptions()
	}

	// Initialize export modal
	v.exportModal = components.NewFormModal("Export Messages")
	v.exportModal.MaxHeight = unit.Dp(500)
	v.exportModal.MaxWidth = unit.Dp(450)
	v.exportFormatSelect = components.NewDropDown(
		components.NewDropDownOption("JSON").WithValue("json"),
		components.NewDropDownOption("CSV").WithValue("csv"),
	)
	v.exportFilterInput = components.NewLabeledInputFieldWithPosition("Filter", "Subject pattern (optional)", components.LabelPositionTop)
	v.exportModal.ReturnFocus = &v.exportBtn
	v.exportModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(14), "Export Format")
				lbl.Color = th.TextColor
				return lbl.Layout(cgtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.exportFormatSelect.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.exportFilterInput.Layout(cgtx, th)
			}),
		)
	}
	v.exportModal.CustomFocusTags = []event.Tag{
		v.exportFormatSelect.FocusTag(),
		v.exportFilterInput.FocusTag(),
	}
	v.exportModal.OnSave = func() bool {
		return v.handleExportMessages()
	}

	v.allFilter.SetSelected(true)
	v.Split.Resize.Ratio = 0.5
	v.mainSplit.Resize.Ratio = 0.2
	v.mainSplit.BarWidth = unit.Dp(2)

	v.SearchEditor.Placeholder = "Search messages..."
	v.SearchEditor.SetIcon(icons.ActionSearch, components.IconPositionStart)
	return v
}

func (v *PubSubView) SetApp(app App) {
	v.App = app
	v.Table.OnCopyFeedback = func(text string) {
		if v.App != nil {
			v.App.ShowToast("Copied: "+text, components.ToastTypeSuccess)
		}
	}
}

func (v *PubSubView) subscribeJetStream(config JetStreamSubscriptionConfig) {
	if v.App == nil {
		return
	}
	client := v.App.GetNatsClient()
	if client == nil {
		v.App.ShowToast("Not connected", components.ToastTypeError)
		return
	}

	js := client.GetJetStream()
	if js == nil {
		v.App.ShowToast("JetStream not enabled", components.ToastTypeError)
		return
	}

	go func() {
		ctx := context.Background()

		// First, find a stream that matches the subject
		streams, err := client.ListStreams(ctx)
		if err != nil {
			v.App.ShowToast("Failed to list streams: "+err.Error(), components.ToastTypeError)
			return
		}

		var matchingStream string
		for _, stream := range streams {
			for _, subj := range stream.Config.Subjects {
				if subjectMatches(config.Subject, subj) {
					matchingStream = stream.Config.Name
					break
				}
			}
			if matchingStream != "" {
				break
			}
		}

		if matchingStream == "" {
			v.App.ShowToast("No stream found matching subject: "+config.Subject, components.ToastTypeError)
			return
		}

		var deliverPolicy jetstream.DeliverPolicy
		switch config.DeliverPolicy {
		case "new":
			deliverPolicy = jetstream.DeliverNewPolicy
		case "last":
			deliverPolicy = jetstream.DeliverLastPolicy
		case "by_start_sequence":
			deliverPolicy = jetstream.DeliverByStartSequencePolicy
		default:
			deliverPolicy = jetstream.DeliverAllPolicy
		}

		var ackPolicy jetstream.AckPolicy
		switch config.AckPolicy {
		case "none":
			ackPolicy = jetstream.AckNonePolicy
		case "all":
			ackPolicy = jetstream.AckAllPolicy
		default:
			ackPolicy = jetstream.AckExplicitPolicy
		}

		consumerConfig := jetstream.ConsumerConfig{
			Durable:       config.Durable,
			FilterSubject: config.Subject,
			DeliverPolicy: deliverPolicy,
			AckPolicy:     ackPolicy,
			HeadersOnly:   config.HeadersOnly,
			MaxDeliver:    config.MaxDeliver,
		}

		if config.DeliverPolicy == "by_start_sequence" && config.StartSeq > 0 {
			consumerConfig.OptStartSeq = config.StartSeq
		}

		cons, err := client.CreateConsumerWithConfig(ctx, matchingStream, consumerConfig)
		if err != nil {
			v.App.ShowToast("Failed to create consumer: "+err.Error(), components.ToastTypeError)
			return
		}

		consCtx, err := cons.Consume(func(msg jetstream.Msg) {
			metadata, err := msg.Metadata()
			var msgType string
			if err == nil {
				msgType = fmt.Sprintf("JS Received [Seq: %d]", metadata.Sequence.Stream)
			} else {
				msgType = "JS Received"
			}
			if config.HeadersOnly {
				msgType += " [Headers Only]"
			}
			v.addMessage(msg.Subject(), msgType, string(msg.Data()))
			msg.Ack()
		})
		if err != nil {
			v.App.ShowToast("Failed to consume: "+err.Error(), components.ToastTypeError)
			return
		}

		v.mu.Lock()
		v.currentJSConsCtx = consCtx
		v.currentJSConsumer = config.Durable
		v.currentJSStream = matchingStream
		v.mu.Unlock()

		v.App.ShowToast("JetStream subscribed to "+config.Subject+" [Consumer: "+config.Durable+", Stream: "+matchingStream+"]", components.ToastTypeSuccess)
		if v.App.GetCurrentPageID() == navigator.PubSubPageId {
			v.App.Invalidate()
		}
	}()
}

// subjectMatches checks if a subject matches a pattern (supports wildcards)
func subjectMatches(subject, pattern string) bool {
	if pattern == ">" {
		return true
	}
	if pattern == subject {
		return true
	}

	parts := strings.Split(subject, ".")
	patternParts := strings.Split(pattern, ".")

	for i, part := range patternParts {
		if part == ">" {
			return true
		}
		if part == "*" {
			if i >= len(parts) {
				return false
			}
			continue
		}
		if i >= len(parts) || parts[i] != part {
			return false
		}
	}

	return len(parts) == len(patternParts)
}

func (v *PubSubView) unsubscribeJetStream() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.currentJSConsCtx != nil {
		v.currentJSConsCtx.Stop()
		v.currentJSConsCtx = nil
	}

	if v.currentJSConsumer != "" && v.App != nil && v.currentJSStream != "" {
		client := v.App.GetNatsClient()
		if client != nil {
			js := client.GetJetStream()
			if js != nil {
				ctx := context.Background()
				_ = js.DeleteConsumer(ctx, v.currentJSStream, v.currentJSConsumer)
			}
		}
		v.App.ShowToast("JetStream unsubscribed [Consumer: "+v.currentJSConsumer+"]", components.ToastTypeInfo)
	}
	v.currentJSConsumer = ""
	v.currentJSStream = ""
}

func (v *PubSubView) isModalVisible() bool {
	return v.showJSSubModal
}

func (v *PubSubView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *PubSubView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.PubSubPageId,
		Title: "Pub/Sub",
		Icon:  icons.ContentSend,
	}
}

func (v *PubSubView) OnEnter() {
	v.filterMessages()
}

func (v *PubSubView) FirstFocusTag() any {
	return v.subjectEditor.FocusTag()
}

func (v *PubSubView) LastFocusTag() any {
	return v.payloadEditor.FocusTag()
}

func (v *PubSubView) OnLeave() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.sub != nil {
		v.sub.Unsubscribe()
		v.sub = nil
	}
	// Unsubscribe from JetStream
	if v.currentJSConsCtx != nil {
		v.currentJSConsCtx.Stop()
		v.currentJSConsCtx = nil
	}
	if v.currentJSConsumer != "" && v.App != nil && v.currentJSStream != "" {
		client := v.App.GetNatsClient()
		if client != nil {
			js := client.GetJetStream()
			if js != nil {
				ctx := context.Background()
				_ = js.DeleteConsumer(ctx, v.currentJSStream, v.currentJSConsumer)
			}
		}
	}
	v.currentJSConsumer = ""
	v.currentJSStream = ""
	// Stop all reply services
	for subject, sub := range v.replyActiveServices {
		sub.Unsubscribe()
		delete(v.replyActiveServices, subject)
	}
	v.replyServiceList = []string{}
}

func (v *PubSubView) filterMessagesLocked() {
	v.filtered = make([]*MessageEntry, 0)
	query := strings.ToLower(v.SearchEditor.GetText())
	for _, msg := range v.messageHistory {
		matchesSearch := query == "" ||
			strings.Contains(strings.ToLower(msg.Subject), query) ||
			strings.Contains(strings.ToLower(msg.Type), query) ||
			strings.Contains(strings.ToLower(msg.Payload), query)

		if !matchesSearch {
			continue
		}

		if v.allFilter.Selected {
			v.filtered = append(v.filtered, msg)
		} else if v.sentFilter.Selected && msg.Type == "Published" {
			v.filtered = append(v.filtered, msg)
		} else if v.recvFilter.Selected && msg.Type == "Received" {
			v.filtered = append(v.filtered, msg)
		}
	}
	totalPages := (len(v.filtered) + v.PerPage - 1) / v.PerPage
	v.Paginator.SetTotalPages(totalPages)
	v.Table.ResetWidths()
}

func (v *PubSubView) filterMessages() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.filterMessagesLocked()
}

func (v *PubSubView) addMessageLocked(msgSubject, msgType, payload string) {
	v.messageHistory = append([]*MessageEntry{{
		Subject: msgSubject,
		Type:    msgType,
		Time:    time.Now().Format("15:04:05"),
		Size:    fmt.Sprintf("%d B", len(payload)),
		Status:  "Success",
		Payload: payload,
	}}, v.messageHistory...)

	switch msgType {
	case "Published", "Request":
		v.messagesSent++
	case "Received":
		v.messagesRecv++
	}

	v.EmptyState = false
	v.filterMessagesLocked()
	if v.App != nil && v.App.GetCurrentPageID() == navigator.PubSubPageId {
		v.App.Invalidate()
	}
}

func (v *PubSubView) addMessage(msgSubject, msgType, payload string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.addMessageLocked(msgSubject, msgType, payload)
}

func (v *PubSubView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.RestoreListFocus {
		v.RestoreListFocus = false
		gtx.Execute(key.FocusCmd{Tag: v.Table.FocusTag()})
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

	v.mu.Lock()
	defer v.mu.Unlock()

	for v.pubBtn.Clicked(gtx) {
		subject := v.subjectEditor.GetText()
		payload := v.codeEditor.GetText()
		if subject == "" {
			if v.App != nil {
				v.App.ShowToast("Subject is required", components.ToastTypeError)
			}
			continue
		}
		if v.App != nil {
			client := v.App.GetNatsClient()
			if client == nil {
				v.App.ShowToast("Not connected", components.ToastTypeError)
				continue
			}

			countStr := v.countInput.GetText()
			count := 1
			if countStr != "" {
				fmt.Sscanf(countStr, "%d", &count)
			}
			if count < 1 {
				count = 1
			}
			if count > 10000 {
				count = 10000 // Limit to prevent abuse
			}

			go func(subj, pay string, msgCount int) {
				for i := 0; i < msgCount; i++ {
					// Process template variables
					processedPayload := v.processTemplate(pay, i+1)

					err := client.Publish(subj, []byte(processedPayload))
					if err != nil {
						v.App.ShowToast(err.Error(), components.ToastTypeError)
						break
					} else {
						v.mu.Lock()
						if msgCount == 1 {
							v.addMessageLocked(subj, "Published", processedPayload)
						}
						v.mu.Unlock()
					}

					// Small delay between messages to avoid overwhelming
					if msgCount > 1 && i < msgCount-1 {
						time.Sleep(10 * time.Millisecond)
					}
				}

				if msgCount > 1 {
					v.App.ShowToast(fmt.Sprintf("Published %d messages", msgCount), components.ToastTypeSuccess)
				} else {
					v.App.ShowToast("Published", components.ToastTypeSuccess)
				}
				if v.App.GetCurrentPageID() == navigator.PubSubPageId {
					v.App.Invalidate()
				}
			}(subject, payload, count)
		}
	}

	for v.subBtn.Clicked(gtx) {
		if v.sub != nil {
			go func() {
				v.sub.Unsubscribe()
				v.mu.Lock()
				v.sub = nil
				v.mu.Unlock()
				if v.App != nil {
					v.App.ShowToast("Unsubscribed", components.ToastTypeInfo)
					if v.App.GetCurrentPageID() == navigator.PubSubPageId {
						v.App.Invalidate()
					}
				}
			}()
			continue
		}

		subject := v.subjectEditor.GetText()
		if subject == "" {
			if v.App != nil {
				v.App.ShowToast("Subject is required", components.ToastTypeError)
			}
			continue
		}

		if v.App != nil {
			client := v.App.GetNatsClient()
			if client == nil {
				v.App.ShowToast("Not connected", components.ToastTypeError)
				continue
			}

			go func(subj, queue string) {
				var sub *nats.Subscription
				var err error

				if queue != "" {
					sub, err = client.SubscribeWithQueue(subj, queue, func(msg *nats.Msg) {
						v.addMessage(msg.Subject, "Received [Queue: "+queue+"]", string(msg.Data))
					})
				} else {
					sub, err = client.Subscribe(subj, func(msg *nats.Msg) {
						v.addMessage(msg.Subject, "Received", string(msg.Data))
					})
				}

				if err != nil {
					v.App.ShowToast(err.Error(), components.ToastTypeError)
				} else {
					v.mu.Lock()
					v.sub = sub
					v.mu.Unlock()
					if queue != "" {
						v.App.ShowToast("Subscribed to "+subj+" [Queue: "+queue+"]", components.ToastTypeSuccess)
					} else {
						v.App.ShowToast("Subscribed to "+subj, components.ToastTypeSuccess)
					}
				}
				if v.App.GetCurrentPageID() == navigator.PubSubPageId {
					v.App.Invalidate()
				}
			}(subject, v.queueEditor.GetText())
		}
	}

	for v.jsSubBtn.Clicked(gtx) {
		if v.currentJSConsCtx != nil {
			go func() {
				v.unsubscribeJetStream()
				if v.App != nil {
					if v.App.GetCurrentPageID() == navigator.PubSubPageId {
						v.App.Invalidate()
					}
				}
			}()
			continue
		}

		subject := v.subjectEditor.GetText()
		if subject == "" {
			if v.App != nil {
				v.App.ShowToast("Subject is required", components.ToastTypeError)
			}
			continue
		}

		v.showJSSubModal = true
		// Reset form fields
		v.durableNameInput.SetText("")
		v.deliverPolicyDropDown.SetSelected(0)
		v.ackPolicyDropDown.SetSelected(1) // Explicit is default
		v.headersOnlyBool.Value = false
		v.maxDeliverInput.SetText("-1")
		v.startSeqInput.SetText("0")
	}

	// Handle JS Subscribe modal close button
	for v.jsSubCloseBtn.Clicked(gtx) {
		v.showJSSubModal = false
	}

	// Handle JS Subscribe modal save button
	for v.jsSubSaveBtn.Clicked(gtx) {
		subject := v.subjectEditor.GetText()
		durable := v.durableNameInput.GetText()
		if durable == "" {
			v.App.ShowToast("Durable name is required", components.ToastTypeError)
			continue
		}

		maxDeliver := -1
		if maxDeliverStr := v.maxDeliverInput.GetText(); maxDeliverStr != "" && maxDeliverStr != "-1 (unlimited)" {
			if val, err := strconv.Atoi(maxDeliverStr); err == nil {
				maxDeliver = val
			}
		}

		var startSeq uint64
		if v.deliverPolicyDropDown.GetSelected().Value == "by_start_sequence" {
			if startSeqStr := v.startSeqInput.GetText(); startSeqStr != "" {
				if val, err := strconv.ParseUint(startSeqStr, 10, 64); err == nil {
					startSeq = val
				}
			}
		}

		config := JetStreamSubscriptionConfig{
			Subject:       subject,
			Durable:       durable,
			DeliverPolicy: v.deliverPolicyDropDown.GetSelected().Value,
			AckPolicy:     v.ackPolicyDropDown.GetSelected().Value,
			HeadersOnly:   v.headersOnlyBool.Value,
			MaxDeliver:    maxDeliver,
			StartSeq:      startSeq,
		}

		v.subscribeJetStream(config)
		v.showJSSubModal = false
	}

	// Handle Escape key to close modal
	if v.showJSSubModal {
		for {
			ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				v.showJSSubModal = false
			}
		}
		// Handle Tab key for focus navigation within JS modal
		for {
			ev, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				v.handleJSSubModalTab(gtx, ke.Modifiers.Contain(key.ModShift))
			}
		}
	}

	for v.requestBtn.Clicked(gtx) {
		subject := v.subjectEditor.GetText()
		if subject == "" {
			if v.App != nil {
				v.App.ShowToast("Subject is required", components.ToastTypeError)
			}
			continue
		}
		if v.App != nil {
			client := v.App.GetNatsClient()
			if client == nil {
				v.App.ShowToast("Not connected", components.ToastTypeError)
				continue
			}
		}
		// Show request options modal
		v.requestTimeoutInput.SetText("5s")
		v.requestMaxReplies.SetText("1")
		v.requestModal.Show()
	}

	for v.clearBtn.Clicked(gtx) {
		v.ConfirmModal.Title = "Clear Messages"
		v.ConfirmModal.Content = "Are you sure you want to clear all messages from history?"
		v.ConfirmModal.ReturnFocus = true
		v.ConfirmModal.SetOnClose(func() {
			v.RestoreListFocus = true
		})
		v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
			if option == "Confirm" {
				v.messageHistory = []*MessageEntry{}
				v.messagesSent = 0
				v.messagesRecv = 0
				v.EmptyState = true
				v.filterMessagesLocked()
				if v.App != nil {
					v.App.ShowToast("Messages cleared", components.ToastTypeSuccess)
				}
			}
		})
		v.ConfirmModal.Show()
	}

	for v.exportBtn.Clicked(gtx) {
		v.exportFilterInput.SetText("")
		v.exportModal.Show()
	}

	for v.replyBtn.Clicked(gtx) {
		subject := v.replySubjectEditor.GetText()
		if subject == "" {
			if v.App != nil {
				v.App.ShowToast("Reply subject pattern is required", components.ToastTypeError)
			}
			continue
		}

		// Check if already serving this subject
		if _, exists := v.replyActiveServices[subject]; exists {
			// Stop the service
			go func(subj string) {
				v.mu.Lock()
				if sub, ok := v.replyActiveServices[subj]; ok {
					sub.Unsubscribe()
					delete(v.replyActiveServices, subj)
					// Remove from list
					for i, s := range v.replyServiceList {
						if s == subj {
							v.replyServiceList = append(v.replyServiceList[:i], v.replyServiceList[i+1:]...)
							break
						}
					}
					v.mu.Unlock()
					if v.App != nil {
						v.App.ShowToast("Reply service stopped: "+subj, components.ToastTypeInfo)
					}
				} else {
					v.mu.Unlock()
				}
			}(subject)
			continue
		}

		if v.App != nil {
			client := v.App.GetNatsClient()
			if client == nil {
				v.App.ShowToast("Not connected", components.ToastTypeError)
				continue
			}

			go func() {
				sub, err := client.Subscribe(subject, func(msg *nats.Msg) {
					v.handleReplyMessage(msg)
				})
				if err != nil {
					v.App.ShowToast("Failed to start reply service: "+err.Error(), components.ToastTypeError)
				} else {
					v.mu.Lock()
					v.replyActiveServices[subject] = sub
					v.replyServiceList = append(v.replyServiceList, subject)
					v.mu.Unlock()
					v.App.ShowToast("Reply service started: "+subject, components.ToastTypeSuccess)
				}
			}()
		}
	}

	if v.SearchEditor.Changed() {
		v.LastFilterTime = gtx.Now
	}

	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
			v.filterMessages()
			v.LastFilterTime = time.Time{}
		} else {
			gtx.Execute(op.InvalidateCmd{At: v.LastFilterTime.Add(300 * time.Millisecond)})
		}
	}

	for v.allFilter.Click.Clicked(gtx) {
		v.allFilter.SetSelected(true)
		v.sentFilter.SetSelected(false)
		v.recvFilter.SetSelected(false)
		v.filterMessagesLocked()
	}

	for v.sentFilter.Click.Clicked(gtx) {
		v.sentFilter.SetSelected(true)
		v.allFilter.SetSelected(false)
		v.recvFilter.SetSelected(false)
		v.filterMessagesLocked()
	}

	for v.recvFilter.Click.Clicked(gtx) {
		v.recvFilter.SetSelected(true)
		v.allFilter.SetSelected(false)
		v.sentFilter.SetSelected(false)
		v.filterMessagesLocked()
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

	// Always update payload editor when selection is valid, ensuring content is in sync
	// Only update if content changed to preserve cursor position
	if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
		selectedPayload := v.filtered[v.SelectedIdx].Payload
		if v.payloadEditor.GetCode() != selectedPayload {
			v.payloadEditor.SetCode(selectedPayload)
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
						return v.layoutActions(cccgtx, th)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Flexed(1, func(cccgtx layout.Context) layout.Dimensions {
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
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.showJSSubModal {
				return v.layoutJSSubModal(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.exportModal.Visible {
				return v.exportModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.requestModal.Visible {
				return v.requestModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
	)
}

func (v *PubSubView) handleTab(gtx layout.Context, shift bool) {
	tags := []any{
		&v.pubBtn,
		&v.subBtn,
		&v.jsSubBtn,
		&v.requestBtn,
		&v.replyBtn,
		&v.clearBtn,
		&v.exportBtn,
		v.SearchEditor.FocusTag(),
		v.allFilter.FocusTag(),
		v.sentFilter.FocusTag(),
		v.recvFilter.FocusTag(),
		v.subjectEditor.FocusTag(),
		v.queueEditor.FocusTag(),
		v.countInput.FocusTag(),
		v.codeEditor.FocusTag(),
	}

	if !v.EmptyState && len(v.filtered) > 0 {
		tags = append(tags,
			v.Table.FocusTag(),
			v.Paginator.PrevClick,
			v.Paginator.NextClick,
		)
	}

	if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
		tags = append(tags, v.payloadEditor.FocusTag())
	}

	HandleTab(gtx, shift, tags, v.next, v.prev)
}

func (v *PubSubView) handleJSSubModalTab(gtx layout.Context, shift bool) {
	tags := []any{
		v.durableNameInput.FocusTag(),
		v.deliverPolicyDropDown.FocusTag(),
		v.ackPolicyDropDown.FocusTag(),
	}

	deliverPolicy := v.deliverPolicyDropDown.GetSelected()
	if deliverPolicy != nil && deliverPolicy.Value == "by_start_sequence" {
		tags = append(tags, v.startSeqInput.FocusTag())
	}

	tags = append(tags,
		v.maxDeliverInput.FocusTag(),
		v.headersOnlyCheck.FocusTag(),
		&v.jsSubCloseBtn,
		&v.jsSubSaveBtn,
	)

	HandleTab(gtx, shift, tags, nil, nil)
}

func (v *PubSubView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(24), "Publish & Subscribe")
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

func (v *PubSubView) layoutActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	pubBtn := components.Button(th, &v.pubBtn, icons.ContentSend, components.IconPositionStart, "Publish")

	subLabel := "Subscribe"
	if v.sub != nil {
		subLabel = "Unsubscribe"
	}
	subBtn := components.Button(th, &v.subBtn, icons.ActionVisibility, components.IconPositionStart, subLabel)

	// JS Subscribe button - shows Unsubscribe when active
	jsSubLabel := "JS Subscribe"
	if v.currentJSConsCtx != nil {
		jsSubLabel = "Unsubscribe"
	}
	jsSubBtn := components.Button(th, &v.jsSubBtn, icons.ActionVisibility, components.IconPositionStart, jsSubLabel)

	requestBtn := components.SecondaryButton(th, &v.requestBtn, icons.ContentSend, components.IconPositionStart, "Request")
	replyBtn := components.SecondaryButton(th, &v.replyBtn, icons.ActionSettings, components.IconPositionStart, "Reply Service")
	clearBtn := components.SecondaryButton(th, &v.clearBtn, icons.ActionDelete, components.IconPositionStart, "Clear")
	exportBtn := components.SecondaryButton(th, &v.exportBtn, icons.FileFileDownload, components.IconPositionStart, "Export")

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return pubBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return subBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return jsSubBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return requestBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return replyBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return clearBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return exportBtn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.SearchEditor.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.allFilter.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(1)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.sentFilter.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(1)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.recvFilter.Layout(cgtx, th)
		}),
	)
}

func (v *PubSubView) layoutCompose(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Subject")
			lbl.Font.Weight = font.Bold
			lbl.Color = th.TextColor
			return layout.Inset{Bottom: unit.Dp(4)}.Layout(cgtx, lbl.Layout)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.subjectEditor.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Queue Group (optional)")
			lbl.Font.Weight = font.Bold
			lbl.Color = th.TextColor
			return layout.Inset{Bottom: unit.Dp(4)}.Layout(cgtx, lbl.Layout)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.queueEditor.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Message Count (for templates: {{.Count}}, {{.TimeStamp}}, {{.UUID}})")
			lbl.Font.Weight = font.Bold
			lbl.Color = th.TextColor
			return layout.Inset{Bottom: unit.Dp(4)}.Layout(cgtx, lbl.Layout)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.countInput.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Payload")
			lbl.Font.Weight = font.Bold
			lbl.Color = th.TextColor
			return layout.Inset{Bottom: unit.Dp(4)}.Layout(cgtx, lbl.Layout)
		}),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.codeEditor.Layout(cgtx, th)
		}),
	)
}

func (v *PubSubView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return v.mainSplit.Layout(gtx, th,
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutCompose(cgtx, th)
		},
		func(cgtx layout.Context) layout.Dimensions {
			if v.EmptyState {
				return v.layoutEmptyState(cgtx, th)
			}

			return v.Split.Layout(cgtx, th,
				func(cccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return v.layoutMessageStats(c4gtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
						layout.Flexed(1, func(c4gtx layout.Context) layout.Dimensions {
							return v.layoutMessageHistory(c4gtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return v.Paginator.Layout(c4gtx, th)
						}),
					)
				},
				func(cccgtx layout.Context) layout.Dimensions {
					return v.layoutMessageDetails(cccgtx, th)
				},
			)
		},
	)
}

func (v *PubSubView) layoutMessageStats(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(18), "Message Statistics")
			header.Color = th.TextColor
			header.Font.Weight = font.Bold
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(cgtx, header.Layout)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return components.StatCard{
						Title: "Messages Sent",
						Value: fmt.Sprintf("%d", v.messagesSent),
					}.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return components.StatCard{
						Title: "Messages Received",
						Value: fmt.Sprintf("%d", v.messagesRecv),
					}.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return components.StatCard{
						Title: "Total Messages",
						Value: fmt.Sprintf("%d", v.messagesSent+v.messagesRecv),
					}.Layout(ccgtx, th)
				}),
			)
		}),
	)
}

func (v *PubSubView) layoutMessageHistory(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	startIdx := (v.Paginator.CurrentPage - 1) * v.PerPage
	endIdx := startIdx + v.PerPage
	if endIdx > len(v.filtered) {
		endIdx = len(v.filtered)
	}
	if startIdx < 0 || startIdx >= len(v.filtered) {
		startIdx = 0
		endIdx = 0
	}

	var pageMessages []*MessageEntry
	if endIdx > startIdx {
		pageMessages = v.filtered[startIdx:endIdx]
	}

	v.Table.Rows = make([]components.TableRow, len(pageMessages))
	for i, msg := range pageMessages {
		v.Table.Rows[i] = components.TableRow{
			Values: []string{
				msg.Subject,
				msg.Type,
				msg.Time,
				msg.Size,
				msg.Status,
			},
			Selected: (startIdx + i) == v.SelectedIdx,
		}
	}

	return v.Table.Layout(gtx, th)
}

func (v *PubSubView) layoutEmptyState(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return components.EmptyState{
		Icon:    icons.ContentSend,
		Title:   "Pub/Sub Tools",
		Message: "Use the tools above to publish, subscribe, or send requests to NATS subjects.",
	}.Layout(gtx, th)
}

func (v *PubSubView) layoutMessageDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(16), "Select a message to view details")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	msg := v.filtered[v.SelectedIdx]

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(18), "Message Details")
				lbl.Font.Weight = font.Bold
				lbl.Color = th.TextColor
				return lbl.Layout(ccgtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Subject", msg.Subject)
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Type", msg.Type)
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Time", msg.Time)
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Size", msg.Size)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(16), "Payload")
				lbl.Font.Weight = font.Bold
				lbl.Color = th.TextColor
				return lbl.Layout(ccgtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				return v.payloadEditor.Layout(ccgtx, th)
			}),
		)
	})
}

// handleReplyMessage processes incoming request messages and sends replies
func (v *PubSubView) handleReplyMessage(msg *nats.Msg) {
	if msg.Reply == "" {
		return // No reply subject, nothing to do
	}

	var response []byte

	switch v.replyTypeSelected {
	case 0: // Static response
		response = []byte(v.replyPayloadEditor.GetText())
	case 1: // Template response
		response = v.processTemplateResponse(msg.Subject)
	case 2: // Command response
		response = v.executeCommandResponse(msg)
	}

	if v.App != nil {
		client := v.App.GetNatsClient()
		if client != nil {
			client.Publish(msg.Reply, response)
			v.addMessage(msg.Subject, "Reply Sent", string(response))
		}
	}
}

// processTemplateResponse processes template placeholders like {{1}}, {{2}} from subject tokens
func (v *PubSubView) processTemplateResponse(subject string) []byte {
	template := v.replyPayloadEditor.GetText()
	tokens := strings.Split(subject, ".")

	result := template
	for i, token := range tokens {
		placeholder := fmt.Sprintf("{{%d}}", i+1)
		result = strings.ReplaceAll(result, placeholder, token)
	}

	return []byte(result)
}

// executeCommandResponse executes a shell command and returns the output
func (v *PubSubView) executeCommandResponse(msg *nats.Msg) []byte {
	cmdStr := v.replyCommandEditor.GetText()
	if cmdStr == "" {
		return []byte("No command configured")
	}

	// Simple command execution (for security, this should be restricted)
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return []byte("Invalid command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return []byte(fmt.Sprintf("Command error: %v", err))
	}

	return output
}

// processTemplate processes template variables for publish
// Supported variables: {{.Count}}, {{.TimeStamp}}, {{.UUID}}
func (v *PubSubView) processTemplate(payload string, count int) string {
	result := payload

	// Replace {{.Count}} with the message number
	result = strings.ReplaceAll(result, "{{.Count}}", fmt.Sprintf("%d", count))

	// Replace {{.TimeStamp}} with current timestamp
	result = strings.ReplaceAll(result, "{{.TimeStamp}}", time.Now().Format(time.RFC3339))

	// Replace {{.UUID}} with a simple unique ID (timestamp + count)
	result = strings.ReplaceAll(result, "{{.UUID}}", fmt.Sprintf("%d-%d", time.Now().UnixNano(), count))

	return result
}

// layoutJSSubModal renders the JetStream subscribe modal
func (v *PubSubView) layoutJSSubModal(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			return components.Clickable(cgtx, &v.jsSubCloseBtn, 0, func(cgtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: cgtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			cgtx.Constraints.Max.X = cgtx.Dp(unit.Dp(600))
			cgtx.Constraints.Min.X = cgtx.Constraints.Max.X

			return components.Card{
				Title: "JetStream Subscribe",
				Inset: layout.UniformInset(unit.Dp(24)),
			}.Layout(cgtx, th, func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutJSSubContent(ccgtx, th)
			})
		}),
	)
}

// layoutJSSubContent renders the content of the JetStream subscribe modal
func (v *PubSubView) layoutJSSubContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	deliverPolicy := v.deliverPolicyDropDown.GetSelected()
	showStartSeq := deliverPolicy != nil && deliverPolicy.Value == "by_start_sequence"

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Subject display
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					lbl := material.Label(th.Material(), unit.Sp(14), "Subject")
					lbl.Font.Weight = font.Bold
					lbl.Color = th.TextColor
					return layout.Inset{Bottom: unit.Dp(4)}.Layout(ccgtx, lbl.Layout)
				}),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					subject := v.subjectEditor.GetText()
					lbl := material.Label(th.Material(), unit.Sp(14), subject)
					lbl.Color = th.SecondaryTextColor
					return lbl.Layout(ccgtx)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		// Durable name
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					lbl := material.Label(th.Material(), unit.Sp(14), "Durable Name *")
					lbl.Font.Weight = font.Bold
					lbl.Color = th.TextColor
					return layout.Inset{Bottom: unit.Dp(4)}.Layout(ccgtx, lbl.Layout)
				}),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return v.durableNameInput.Layout(ccgtx, th)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		// Deliver Policy and Ack Policy in a row
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cgtx,
				layout.Flexed(0.48, func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							lbl := material.Label(th.Material(), unit.Sp(14), "Deliver Policy")
							lbl.Font.Weight = font.Bold
							lbl.Color = th.TextColor
							return layout.Inset{Bottom: unit.Dp(4)}.Layout(cccgtx, lbl.Layout)
						}),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.deliverPolicyDropDown.Layout(cccgtx, th)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Flexed(0.48, func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							lbl := material.Label(th.Material(), unit.Sp(14), "Ack Policy")
							lbl.Font.Weight = font.Bold
							lbl.Color = th.TextColor
							return layout.Inset{Bottom: unit.Dp(4)}.Layout(cccgtx, lbl.Layout)
						}),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.ackPolicyDropDown.Layout(cccgtx, th)
						}),
					)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		// Start Sequence (only shown when ByStartSeq is selected)
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !showStartSeq {
				return layout.Dimensions{}
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					lbl := material.Label(th.Material(), unit.Sp(14), "Start Sequence")
					lbl.Font.Weight = font.Bold
					lbl.Color = th.TextColor
					return layout.Inset{Bottom: unit.Dp(4)}.Layout(ccgtx, lbl.Layout)
				}),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return v.startSeqInput.Layout(ccgtx, th)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		// Max Deliver and Headers Only
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cgtx,
				layout.Flexed(0.48, func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							lbl := material.Label(th.Material(), unit.Sp(14), "Max Deliver")
							lbl.Font.Weight = font.Bold
							lbl.Color = th.TextColor
							return layout.Inset{Bottom: unit.Dp(4)}.Layout(cccgtx, lbl.Layout)
						}),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.maxDeliverInput.Layout(cccgtx, th)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Flexed(0.48, func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(ccgtx,
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.headersOnlyCheck.Layout(cccgtx)
						}),
					)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
		// Buttons
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					btn := components.SecondaryButton(th, &v.jsSubCloseBtn, nil, 0, "Cancel")
					return btn.Layout(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					btn := components.Button(th, &v.jsSubSaveBtn, nil, 0, "Subscribe")
					return btn.Layout(ccgtx, th)
				}),
			)
		}),
	)
}

func (v *PubSubView) handleExportMessages() bool {
	if len(v.filtered) == 0 {
		if v.App != nil {
			v.App.ShowToast("No messages to export", components.ToastTypeError)
		}
		return false
	}

	format := v.exportFormatSelect.GetSelected().Value
	filterPattern := strings.ToLower(v.exportFilterInput.GetText())

	v.mu.Lock()
	messages := make([]*MessageEntry, len(v.filtered))
	copy(messages, v.filtered)
	v.mu.Unlock()

	var filtered []*MessageEntry
	if filterPattern != "" {
		for _, msg := range messages {
			if strings.Contains(strings.ToLower(msg.Subject), filterPattern) {
				filtered = append(filtered, msg)
			}
		}
	} else {
		filtered = messages
	}

	if len(filtered) == 0 {
		if v.App != nil {
			v.App.ShowToast("No messages match the filter", components.ToastTypeError)
		}
		return false
	}

	go func() {
		var data []byte
		var ext string
		var err error

		switch format {
		case "json":
			data, err = json.MarshalIndent(filtered, "", "  ")
			ext = "json"
		case "csv":
			var buf bytes.Buffer
			writer := csv.NewWriter(&buf)
			if err := writer.Write([]string{"Subject", "Type", "Time", "Size", "Status", "Payload"}); err != nil {
				if v.App != nil {
					v.App.ShowToast("Failed to write CSV header: "+err.Error(), components.ToastTypeError)
				}
				return
			}
			for _, msg := range filtered {
				if err := writer.Write([]string{msg.Subject, msg.Type, msg.Time, msg.Size, msg.Status, msg.Payload}); err != nil {
					if v.App != nil {
						v.App.ShowToast("Failed to write CSV row: "+err.Error(), components.ToastTypeError)
					}
					return
				}
			}
			writer.Flush()
			data = buf.Bytes()
			ext = "csv"
		default:
			if v.App != nil {
				v.App.ShowToast("Unknown export format", components.ToastTypeError)
			}
			return
		}

		if err != nil {
			if v.App != nil {
				v.App.ShowToast("Failed to export: "+err.Error(), components.ToastTypeError)
			}
			return
		}

		// Open save file dialog
		filename := fmt.Sprintf("nats_messages_%s.%s", time.Now().Format("20060102_150405"), ext)
		savePath, err := dialog.File().Title("Save Messages").SetStartFile(filename).Save()
		if err != nil {
			// User cancelled or error occurred
			return
		}

		if err := os.WriteFile(savePath, data, 0644); err != nil {
			if v.App != nil {
				v.App.ShowToast("Failed to write file: "+err.Error(), components.ToastTypeError)
			}
			return
		}

		if v.App != nil {
			v.App.ShowToast(fmt.Sprintf("Exported %d messages to %s", len(filtered), filepath.Base(savePath)), components.ToastTypeSuccess)
			log.Logger().Info().
				Str("format", format).
				Int("total_messages", len(messages)).
				Int("exported_messages", len(filtered)).
				Str("filter_pattern", filterPattern).
				Str("file_path", savePath).
				Str("filename", filepath.Base(savePath)).
				Msg("Messages exported")
		}
	}()

	return true
}

func (v *PubSubView) handleRequestWithOptions() bool {
	subject := v.subjectEditor.GetText()
	payload := v.codeEditor.GetText()

	if subject == "" {
		if v.App != nil {
			v.App.ShowToast("Subject is required", components.ToastTypeError)
		}
		return false
	}

	// Parse timeout
	timeoutStr := v.requestTimeoutInput.GetText()
	if timeoutStr == "" {
		timeoutStr = "5s"
	}
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		if v.App != nil {
			v.App.ShowToast("Invalid timeout format: "+err.Error(), components.ToastTypeError)
		}
		return false
	}

	// Parse max replies
	maxRepliesStr := v.requestMaxReplies.GetText()
	if maxRepliesStr == "" {
		maxRepliesStr = "1"
	}
	maxReplies, err := strconv.Atoi(maxRepliesStr)
	if err != nil || maxReplies < 1 {
		if v.App != nil {
			v.App.ShowToast("Max replies must be a positive number", components.ToastTypeError)
		}
		return false
	}

	client := v.App.GetNatsClient()
	if client == nil {
		if v.App != nil {
			v.App.ShowToast("Not connected", components.ToastTypeError)
		}
		return false
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		nc := client.Conn()
		if nc == nil {
			v.App.ShowToast("Not connected", components.ToastTypeError)
			return
		}

		// Send request
		msg, err := nc.RequestWithContext(ctx, subject, []byte(payload))
		if err != nil {
			v.App.ShowToast(err.Error(), components.ToastTypeError)
			return
		}

		// Add the reply to message history
		v.addMessage(subject, "Request", payload)
		v.addMessage(msg.Subject, "Received", string(msg.Data))

		var replyCount int
		if maxReplies == 1 {
			v.App.ShowToast("Response received", components.ToastTypeSuccess)
			replyCount = 1
		} else {
			// For multiple replies, subscribe to the reply subject
			replySubject := msg.Subject
			replyCount = 1 // Already got one

			sub, err := nc.SubscribeSync(replySubject)
			if err != nil {
				v.App.ShowToast(fmt.Sprintf("First reply received, failed to subscribe for more: %v", err), components.ToastTypeWarning)
				return
			}
			defer sub.Unsubscribe()

			for replyCount < maxReplies {
				nextMsg, err := sub.NextMsg(timeout)
				if err != nil {
					break // Timeout or error, stop collecting
				}
				v.addMessage(nextMsg.Subject, "Received", string(nextMsg.Data))
				replyCount++
			}

			v.App.ShowToast(fmt.Sprintf("Received %d replies", replyCount), components.ToastTypeSuccess)
		}

		// Log the request operation
		log.Logger().Info().
			Str("subject", subject).
			Int("payload_size", len(payload)).
			Dur("timeout", timeout).
			Int("max_replies", maxReplies).
			Int("replies_received", replyCount).
			Msg("Request-reply completed")

		if v.App != nil {
			v.App.Invalidate()
		}
	}()

	return true
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *PubSubView) HandleShortcuts(gtx layout.Context) bool {
	// Check if any modal is visible first - view shortcuts disabled when modal is open
	if v.isModalVisible() {
		return false
	}

	// View-specific shortcuts - process only ONE event per frame
	ev, ok := gtx.Event(
		key.Filter{Name: key.NameReturn, Optional: key.ModShortcut},
		key.Filter{Name: key.NameEnter, Optional: key.ModShortcut},
		key.Filter{Name: key.Name("C"), Optional: key.ModShortcut},
	)
	if !ok {
		return false
	}

	if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
		switch {
		case (ke.Name == key.NameReturn || ke.Name == key.NameEnter) && ke.Modifiers.Contain(key.ModShortcut):
			// Ctrl+Enter to publish
			v.pubBtn.Click()
			return true
		case ke.Name == key.Name("C") && ke.Modifiers.Contain(key.ModShortcut):
			// Ctrl+C to clear messages
			v.clearBtn.Click()
			return true
		}
	}
	return false
}

// GetShortcutsHelp returns help text for this view's shortcuts
func (v *PubSubView) GetShortcutsHelp() []shortcuts.Shortcut {
	return []shortcuts.Shortcut{
		shortcuts.Custom("Publish", "Publish message", key.NameReturn, key.ModShortcut, nil, func() {}),
		shortcuts.Custom("Clear", "Clear messages", key.Name("C"), key.ModShortcut, nil, func() {}),
		shortcuts.CopyName(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.CopyRow(func() bool { return v.SelectedIdx >= 0 }, func() {}),
	}
}
