package views

import (
	"context"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"

	"gioui.org/io/event"

	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"

	"gioui.org/font"
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

	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/nats"
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
	"github.com/thedataflows/nats-desktop/internal/utils"

	"github.com/nats-io/nats.go/jetstream"
)

type StreamsView struct {
	*BaseView

	// View-specific data
	streams  []*StreamInfo
	filtered []*StreamInfo

	// Extra buttons not in BaseView
	purgeBtn   widget.Clickable
	browseBtn  widget.Clickable
	getSeqBtn  widget.Clickable
	gapsBtn    widget.Clickable
	copyBtn    widget.Clickable
	sealBtn    widget.Clickable
	stateBtn   widget.Clickable
	statsBtn   widget.Clickable
	clusterBtn widget.Clickable

	// Cluster Modal
	clusterModal             *components.FormModal
	selectedStreamForCluster string
	clusterInfo              *jetstream.ClusterInfo
	clusterLoading           bool

	// Stats Modal - using FormModal for display with custom content
	statsModal             *components.FormModal
	selectedStreamForStats string
	streamStats            StreamStatsData

	// Get By Sequence Modal
	getSeqModal *components.FormModal
	seqInput    *components.NumericInputField

	// Direct Get By Subject Modal
	directGetBtn    widget.Clickable
	directGetModal  *components.FormModal
	directGetInput  *components.InputField
	directGetResult *jetstream.RawStreamMsg

	// Copy Stream Modal
	copyModal     *components.FormModal
	copyNameInput *components.InputField

	// Message Gaps Modal
	gapsModal   *components.FormModal
	gapsResults []nats.MessageGap
	gapsLoading bool

	// Create Stream Modal
	createModal *components.FormModal

	// Mandatory fields
	nameInput     *components.InputField
	subjectsInput *components.InputField

	// Optional fields (in expandable section)
	optionalSection     *components.ExpandableSection
	descriptionInput    *components.InputField
	storageTypeDropDown *components.DropDown
	retentionInput      *components.DropDown
	maxMsgsInput        *components.InputField
	maxBytesInput       *components.InputField
	maxAgeInput         *components.InputField
	replicasInput       *components.InputField

	// Advanced options (in separate expandable section)
	advancedSection *components.ExpandableSection

	// Mirror configuration (separate section below)
	mirrorSection       *components.ExpandableSection
	mirrorNameInput     *components.InputField
	mirrorFilterInput   *components.InputField
	mirrorStartSeqInput *components.InputField

	// Sources configuration (comma-separated stream names)
	sourcesInput *components.InputField

	// Republish configuration
	republishSourceInput      *components.InputField
	republishDestInput        *components.InputField
	republishHeadersOnlyBool  widget.Bool
	republishHeadersOnlyCheck components.CheckBoxStyle

	// Subject transforms (on the stream itself)
	transformSourceInput *components.InputField
	transformDestInput   *components.InputField

	// Messages Modal - using reusable component
	messagesModal     *components.MessageViewerModal
	selectedStream    string
	streamMessages    []*jetstream.RawStreamMsg
	pendingModalItems []components.MessageViewerItem
	pendingModalTitle string

	// Infinite scroll state
	streamTotalMessages int64
	streamLastSeq       uint64
	hasMoreMessages     bool

	// Per-stream health check result
	selectedStreamHealth *StreamHealth
	healthLoading        bool

	// State modal - using FormModal
	stateModal   *components.FormModal
	stateResults *jetstream.StreamState
	stateLoading bool

	next, prev any
}

type StreamStatsData struct {
	TotalMessages uint64
	TotalBytes    uint64
	FirstSeq      uint64
	LastSeq       uint64
	NumSubjects   uint64
	NumConsumers  int
	ConsumerStats []ConsumerStat
}

type ConsumerStat struct {
	Name       string
	Pending    int64
	Delivered  int64
	AckPending int64
}

type StreamHealth struct {
	Name           string
	Status         string // "Healthy", "Warning", "Critical"
	Lag            int64
	SourcesHealthy bool
	MirrorHealthy  bool
	ClusterHealthy bool
	MessageCount   int64
	LastSeq        uint64
	Details        string
}

type StreamInfo struct {
	Name      string
	Subjects  string
	Storage   string
	Retention string
	Messages  int64
	Bytes     int64
	Consumers int
	Created   string
	IsSealed  bool
}

func NewStreamsView(th *theme.Theme) *StreamsView {
	v := &StreamsView{
		BaseView: NewBaseView(
			[]string{"Name", "Subjects", "Storage", "Messages", "Bytes", "Consumers", "Created"},
			20,
		),
		streams:          []*StreamInfo{},
		filtered:         []*StreamInfo{},
		messagesModal:    components.NewMessageViewerModal(th),
		createModal:      components.NewFormModal("Create Stream"),
		getSeqModal:      components.NewFormModal("Get Message by Sequence"),
		copyModal:        components.NewFormModal("Copy Stream"),
		gapsModal:        components.NewFormModal("Message Gaps"),
		nameInput:        components.NewLabeledInputFieldWithPosition("Stream name", "", components.LabelPositionTop),
		copyNameInput:    components.NewLabeledInputFieldWithPosition("New stream name", "Enter new name", components.LabelPositionTop),
		subjectsInput:    components.NewLabeledInputFieldWithPosition("Subjects", "Space-separated", components.LabelPositionTop),
		optionalSection:  components.NewExpandableSection("Advanced Options"),
		descriptionInput: components.NewLabeledInputFieldWithPosition("Description", "", components.LabelPositionTop),
		storageTypeDropDown: components.NewLabeledDropDown("Storage type",
			components.NewDropDownOption("File").WithValue("file").DefaultSelected(),
			components.NewDropDownOption("Memory").WithValue("memory"),
		),
		retentionInput: components.NewLabeledDropDown("Retention",
			components.NewDropDownOption("Limits").WithValue("limits").DefaultSelected(),
			components.NewDropDownOption("Interest").WithValue("interest"),
			components.NewDropDownOption("Work Queue").WithValue("workqueue"),
		),
		maxMsgsInput:  components.NewLabeledInputFieldWithPosition("Max messages", "-1 for unlimited", components.LabelPositionTop),
		maxBytesInput: components.NewLabeledInputFieldWithPosition("Max bytes", "-1 for unlimited", components.LabelPositionTop),
		maxAgeInput:   components.NewLabeledInputFieldWithPosition("Max age", "e.g., 24h, 7d", components.LabelPositionTop),
		replicasInput: components.NewLabeledInputFieldWithPosition("Replicas", "1-5", components.LabelPositionTop),
		seqInput:      components.NewLabeledNumericInputFieldWithPosition("Sequence number", "", components.LabelPositionTop),
		// Initialize new modals
		statsModal: components.NewFormModal("Stream Statistics"),
		stateModal: components.NewFormModal("Stream State"),
		// Direct Get by Subject
		directGetModal: components.NewFormModal("Direct Get by Subject"),
		directGetInput: components.NewLabeledInputFieldWithPosition("Subject pattern", "e.g., orders.>", components.LabelPositionTop),
		// Cluster modal
		clusterModal: components.NewFormModal("Stream Cluster Info"),
		// Advanced stream options
		advancedSection:      components.NewExpandableSection("Sources & Republish"),
		mirrorSection:        components.NewExpandableSection("Mirror Configuration"),
		mirrorNameInput:      components.NewLabeledInputFieldWithPosition("Mirror stream name", "Leave empty for none", components.LabelPositionTop),
		mirrorFilterInput:    components.NewLabeledInputFieldWithPosition("Mirror filter subject", "Optional", components.LabelPositionTop),
		mirrorStartSeqInput:  components.NewLabeledInputFieldWithPosition("Mirror start sequence", "Optional", components.LabelPositionTop),
		sourcesInput:         components.NewLabeledInputFieldWithPosition("Source streams", "Comma-separated", components.LabelPositionTop),
		republishSourceInput: components.NewLabeledInputFieldWithPosition("Republish source pattern", "", components.LabelPositionTop),
		republishDestInput:   components.NewLabeledInputFieldWithPosition("Republish destination pattern", "", components.LabelPositionTop),
		transformSourceInput: components.NewLabeledInputFieldWithPosition("Transform source pattern", "", components.LabelPositionTop),
		transformDestInput:   components.NewLabeledInputFieldWithPosition("Transform destination pattern", "", components.LabelPositionTop),
	}

	// Configure BaseView components
	v.SearchEditor.Placeholder = "Search streams..."
	v.SearchEditor.SetIcon(icons.ActionSearch, components.IconPositionStart)

	// Initialize checkbox with tab navigation
	v.republishHeadersOnlyCheck = components.CheckBox(th.Material(), &v.republishHeadersOnlyBool, "Republish headers only")
	v.republishHeadersOnlyCheck.SetTheme(th)
	v.republishHeadersOnlyCheck.SetOnTab(func(gtx layout.Context, shift bool) {
		v.createModal.HandleTabNavigation(gtx, shift)
	})

	// Configure SplitView
	v.Split = components.SplitView{
		Resize: component.Resize{
			Ratio: 0.7,
		},
		BarWidth: unit.Dp(2),
	}

	// Set up messages modal actions (no purge for streams)
	v.messagesModal.SetActions(
		func(item components.MessageViewerItem) {
			v.promptMessageDelete(item.ID)
		},
		nil, // No purge action for streams
		nil, // No history action for stream messages
	)

	// Set up content loader for messages modal
	v.messagesModal.SetOnLoadContent(func(item components.MessageViewerItem) string {
		return item.Content
	})

	// Set up infinite scroll loading
	v.messagesModal.SetOnLoadMore(func() {
		v.loadMoreStreamMessages()
	})

	// Set up confirmation modal visibility checker
	v.messagesModal.IsConfirmationModalVisible = func() bool {
		return v.ConfirmModal.IsVisible()
	}

	// Set up invalidation callback
	v.messagesModal.SetOnInvalidate(func() {
		if v.App != nil {
			v.App.Invalidate()
		}
	})

	// Set up close callback to return focus to parent table
	v.messagesModal.SetOnClose(func() {
		v.RestoreListFocus = true
	})

	// Set up copy feedback callback for messages modal
	v.messagesModal.SetOnCopyFeedback(func(text string) {
		if v.App != nil {
			v.App.ShowToast("Copied: "+text, components.ToastTypeSuccess)
		}
	})

	// Set up create stream modal with custom content
	v.createModal.ReturnFocus = v.Table.FocusTag()
	v.createModal.MaxWidth = unit.Dp(500)
	v.createModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.nameInput.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.subjectsInput.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.optionalSection.Layout(cgtx, th, func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.descriptionInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.storageTypeDropDown.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.retentionInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.maxMsgsInput.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.maxBytesInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.maxAgeInput.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							cccgtx.Constraints.Max.X = cccgtx.Constraints.Max.X / 2
							return v.replicasInput.Layout(cccgtx, th)
						}),
					)
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.advancedSection.Layout(cgtx, th, func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.sourcesInput.Layout(cccgtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.republishSourceInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.republishDestInput.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return v.republishHeadersOnlyCheck.Layout(cccgtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.transformSourceInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.transformDestInput.Layout(ccccgtx, th)
								}),
							)
						}),
					)
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.mirrorSection.Layout(cgtx, th, func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.mirrorNameInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.mirrorFilterInput.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							cccgtx.Constraints.Max.X = cccgtx.Constraints.Max.X / 2
							return v.mirrorStartSeqInput.Layout(cccgtx, th)
						}),
					)
				})
			}),
		)
	}
	// Set up TAB navigation order for the modal - using dynamic function to handle collapsed sections
	v.createModal.CustomFocusTagsFunc = func() []event.Tag {
		tags := []event.Tag{
			v.nameInput.FocusTag(),
			v.subjectsInput.FocusTag(),
			v.optionalSection.FocusTag(),
		}
		// Only include optional section contents if expanded
		if v.optionalSection.Expanded {
			tags = append(tags,
				v.descriptionInput.FocusTag(),
				v.storageTypeDropDown.FocusTag(),
				v.retentionInput.FocusTag(),
				v.maxMsgsInput.FocusTag(),
				v.maxBytesInput.FocusTag(),
				v.maxAgeInput.FocusTag(),
				v.replicasInput.FocusTag(),
			)
		}
		tags = append(tags, v.advancedSection.FocusTag())
		// Only include advanced section contents if expanded
		if v.advancedSection.Expanded {
			tags = append(tags,
				v.sourcesInput.FocusTag(),
				v.republishSourceInput.FocusTag(),
				v.republishDestInput.FocusTag(),
				v.republishHeadersOnlyCheck.FocusTag(),
				v.transformSourceInput.FocusTag(),
				v.transformDestInput.FocusTag(),
			)
		}
		tags = append(tags, v.mirrorSection.FocusTag())
		// Only include mirror section contents if expanded
		if v.mirrorSection.Expanded {
			tags = append(tags,
				v.mirrorNameInput.FocusTag(),
				v.mirrorFilterInput.FocusTag(),
				v.mirrorStartSeqInput.FocusTag(),
			)
		}
		return tags
	}
	v.createModal.OnSave = func() bool {
		return v.handleCreateStream()
	}
	v.createModal.OnClose = func() {
		v.RestoreListFocus = true
	}

	// Set up Get Sequence modal
	v.getSeqModal.ReturnFocus = v.Table.FocusTag()
	v.getSeqModal.MaxWidth = unit.Dp(400)
	v.getSeqModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		// Limit the width of the input field
		gtx.Constraints.Max.X = gtx.Dp(unit.Dp(300))
		return v.seqInput.Layout(gtx, th)
	}
	v.getSeqModal.CustomFocusTags = []event.Tag{
		v.seqInput.FocusTag(),
	}
	v.getSeqModal.OnSave = func() bool {
		return v.handleGetBySequence()
	}
	v.getSeqModal.OnClose = func() {
		v.RestoreListFocus = true
	}

	// Set up Direct Get by Subject modal
	v.directGetModal.ReturnFocus = v.Table.FocusTag()
	v.directGetModal.MaxWidth = unit.Dp(400)
	v.directGetModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		// Limit the width of the input field to match sequence number field
		gtx.Constraints.Max.X = gtx.Dp(unit.Dp(300))
		return v.directGetInput.Layout(gtx, th)
	}
	v.directGetModal.CustomFocusTags = []event.Tag{
		v.directGetInput.FocusTag(),
	}
	v.directGetModal.OnSave = func() bool {
		return v.handleDirectGetBySubject()
	}
	v.directGetModal.OnClose = func() {
		v.RestoreListFocus = true
	}

	// Set up Copy Stream modal
	v.copyModal.ReturnFocus = v.Table.FocusTag()
	v.copyModal.MaxWidth = unit.Dp(400)
	v.copyModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		// Limit the width of the input field to match sequence number field
		gtx.Constraints.Max.X = gtx.Dp(unit.Dp(300))
		return v.copyNameInput.Layout(gtx, th)
	}
	v.copyModal.CustomFocusTags = []event.Tag{
		v.copyNameInput.FocusTag(),
	}
	v.copyModal.OnSave = func() bool {
		return v.handleCopyStream()
	}
	v.copyModal.OnClose = func() {
		v.RestoreListFocus = true
	}

	// Set up Gaps modal - display only
	v.gapsModal.ReturnFocus = v.Table.FocusTag()
	v.gapsModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.layoutGapsContent(gtx, th)
	}
	v.gapsModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.gapsModal.HideSaveButton = true
	v.gapsModal.MaxHeight = unit.Dp(400)

	// Set up Stats modal - display only
	v.statsModal.ReturnFocus = v.Table.FocusTag()
	v.statsModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.layoutStatsContent(gtx, th)
	}
	v.statsModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.statsModal.HideSaveButton = true

	// Set up State modal - display only
	v.stateModal.ReturnFocus = v.Table.FocusTag()
	v.stateModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.layoutStateContent(gtx, th)
	}
	v.stateModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.stateModal.HideSaveButton = true

	// Set up Cluster modal - display only
	v.clusterModal.ReturnFocus = v.Table.FocusTag()
	v.clusterModal.MaxHeight = unit.Dp(400)
	v.clusterModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.layoutClusterContent(gtx, th)
	}
	v.clusterModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.clusterModal.HideSaveButton = true

	return v
}

func (v *StreamsView) SetApp(app App) {
	v.App = app
	v.Table.OnCopyFeedback = func(text string) {
		if v.App != nil {
			v.App.ShowToast("Copied: "+text, components.ToastTypeSuccess)
		}
	}
}

func (v *StreamsView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *StreamsView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.StreamsPageId,
		Title: "Streams",
		Icon:  icons.DeviceStorage,
	}
}

func (v *StreamsView) OnEnter() {
	v.Refresh()
}

func (v *StreamsView) FirstFocusTag() any {
	return v.SearchEditor.FocusTag()
}

func (v *StreamsView) LastFocusTag() any {
	return v.Paginator.NextClick
}

func (v *StreamsView) Refresh() {
	if v.App == nil || v.Loading {
		return
	}

	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		v.streams = []*StreamInfo{}
		v.EmptyState = true
		v.filterStreams()
		return
	}

	v.Loading = true
	go func() {
		defer func() { v.Loading = false }()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		streams, err := client.ListStreams(ctx)
		if err != nil {
			v.App.ShowToast("Failed to list streams: "+err.Error(), components.ToastTypeError)
			return
		}

		newStreams := make([]*StreamInfo, 0, len(streams))
		for _, s := range streams {
			newStreams = append(newStreams, &StreamInfo{
				Name:      s.Config.Name,
				Subjects:  strings.Join(s.Config.Subjects, ", "),
				Storage:   s.Config.Storage.String(),
				Messages:  int64(s.State.Msgs),
				Bytes:     int64(s.State.Bytes),
				Consumers: s.State.Consumers,
				Created:   s.Created.Format("2006-01-02 15:04:05"),
				IsSealed:  s.Config.Sealed,
			})
		}

		v.streams = newStreams
		v.EmptyState = len(newStreams) == 0
		v.filterStreams()
		if v.App != nil && v.App.GetCurrentPageID() == navigator.StreamsPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *StreamsView) filterStreams() {
	query := strings.ToLower(v.SearchEditor.GetText())
	v.filtered = FilterItems(v.streams, query, func(stream *StreamInfo) bool {
		return strings.Contains(strings.ToLower(stream.Name), query) ||
			strings.Contains(strings.ToLower(stream.Subjects), query) ||
			strings.Contains(strings.ToLower(stream.Storage), query)
	})
	totalPages := (len(v.filtered) + v.PerPage - 1) / v.PerPage
	v.Paginator.SetTotalPages(totalPages)
}

func (v *StreamsView) addSampleData() {
	v.streams = []*StreamInfo{
		{
			Name:      "ORDERS",
			Subjects:  "orders.*",
			Storage:   "File",
			Retention: "Limits",
			Messages:  1254890,
			Bytes:     245987123,
			Consumers: 3,
			Created:   "2024-01-15",
		},
		{
			Name:      "EVENTS",
			Subjects:  "events.>",
			Storage:   "Memory",
			Retention: "Interest",
			Messages:  52341,
			Bytes:     12345678,
			Consumers: 1,
			Created:   "2024-01-20",
		},
		{
			Name:      "METRICS",
			Subjects:  "metrics.>",
			Storage:   "File",
			Retention: "WorkQueue",
			Messages:  8901234,
			Bytes:     987654321,
			Consumers: 5,
			Created:   "2024-01-10",
		},
	}
	v.EmptyState = false
}

func (v *StreamsView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.RestoreListFocus {
		v.RestoreListFocus = false
		// If messages modal is open, return focus to it, otherwise to the table
		if v.messagesModal.IsOpen {
			gtx.Execute(key.FocusCmd{Tag: v.messagesModal.FocusTag()})
		} else {
			gtx.Execute(key.FocusCmd{Tag: v.Table.FocusTag()})
		}
	}

	// Only handle TAB navigation when no modal is open
	// The modals have their own TAB handling
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

	for v.AddBtn.Clicked(gtx) {
		v.showCreateStreamModal()
	}

	for v.RefreshBtn.Clicked(gtx) {
		v.Refresh()
	}

	for v.DeleteBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.promptStreamDelete(v.filtered[v.SelectedIdx].Name)
		}
	}

	// Handle Delete key for stream deletion (only when no modal is open)
	if !v.isModalVisible() {
		for {
			ev, ok := gtx.Event(key.Filter{Name: key.NameDeleteForward}, key.Filter{Name: key.NameDeleteBackward})
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
					v.promptStreamDelete(v.filtered[v.SelectedIdx].Name)
				}
			}
		}
	}

	for v.purgeBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.promptStreamPurge(v.filtered[v.SelectedIdx].Name)
		}
	}

	for v.browseBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.loadStreamMessages(v.filtered[v.SelectedIdx].Name)
		}
	}

	for v.getSeqBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.showGetBySequenceModal()
		}
	}

	for v.directGetBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.directGetInput.SetText("")
			v.directGetModal.Show()
		}
	}

	for v.gapsBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.detectMessageGaps(v.filtered[v.SelectedIdx].Name)
		}
	}

	for v.copyBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.copyNameInput.SetText("")
			v.copyModal.Show()
		}
	}

	for v.sealBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.promptSealStream(v.filtered[v.SelectedIdx].Name)
		}
	}

	for v.stateBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.showStreamState(v.filtered[v.SelectedIdx].Name)
		}
	}

	for v.statsBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			streamName := v.filtered[v.SelectedIdx].Name
			v.showStreamStats(streamName)
			log.Logger().Info().
				Str("stream", streamName).
				Str("action", "view_stats").
				Msg("Stream stats viewed")
		}
	}

	for v.clusterBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			streamName := v.filtered[v.SelectedIdx].Name
			v.showStreamCluster(streamName)
			log.Logger().Info().
				Str("stream", streamName).
				Str("action", "view_cluster_info").
				Msg("Stream cluster info viewed")
		}
	}

	if v.SearchEditor.Changed() {
		v.LastFilterTime = gtx.Now
	}

	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
			v.filterStreams()
			v.LastFilterTime = time.Time{}
		} else {
			gtx.Execute(op.InvalidateCmd{At: v.LastFilterTime.Add(300 * time.Millisecond)})
		}
	}

	// Handle pagination button clicks
	if v.Paginator.Next(gtx) {
		v.Paginator.NextPage()
	}
	if v.Paginator.Prev(gtx) {
		v.Paginator.PrevPage()
	}

	// Handle table clicks and selection changes
	clicked := v.Table.Clicked()
	doubleClicked := v.Table.DoubleClicked()
	if clicked || doubleClicked {
		newIdx := (v.Paginator.CurrentPage-1)*v.PerPage + v.Table.SelectedRow
		if doubleClicked {
			if newIdx >= 0 && newIdx < len(v.filtered) {
				v.loadStreamMessages(v.filtered[newIdx].Name)
				v.App.Invalidate()
			}
		}
		if newIdx != v.SelectedIdx {
			v.SelectedIdx = newIdx
			if newIdx >= 0 && newIdx < len(v.filtered) {
				v.checkStreamHealth(v.filtered[newIdx].Name)
			}
		}
	}
	if v.Table.SelectionChanged() {
		newIdx := (v.Paginator.CurrentPage-1)*v.PerPage + v.Table.SelectedRow
		if newIdx != v.SelectedIdx {
			v.SelectedIdx = newIdx
			if newIdx >= 0 && newIdx < len(v.filtered) {
				v.checkStreamHealth(v.filtered[newIdx].Name)
			}
		}
	}

	paint.FillShape(gtx.Ops, th.Palette.Bg, clip.Rect{
		Max: gtx.Constraints.Max,
	}.Op())

	layout.UniformInset(unit.Dp(32)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
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

	// Check for pending modal open (from background goroutine)
	if v.pendingModalItems != nil {
		items := v.pendingModalItems
		title := v.pendingModalTitle
		v.pendingModalItems = nil
		v.pendingModalTitle = ""
		v.messagesModal.SetHasMoreItems(v.hasMoreMessages)
		v.messagesModal.Open(title, items)
	}

	// Layout all modals using Stack to ensure proper layering
	// The last modal in the stack will be on top and capture events
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.messagesModal.IsOpen {
				return v.messagesModal.Layout(cgtx)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.ConfirmModal.IsVisible() {
				return v.ConfirmModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.createModal.Visible {
				return v.createModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.getSeqModal.Visible {
				return v.getSeqModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.directGetModal.Visible {
				return v.directGetModal.Layout(cgtx, th)
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
			if v.gapsModal.Visible {
				return v.gapsModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.statsModal.Visible {
				return v.statsModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.clusterModal.Visible {
				return v.clusterModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.stateModal.Visible {
				return v.stateModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
	)
}

func (v *StreamsView) isModalVisible() bool {
	return v.createModal.Visible || v.messagesModal.IsOpen || v.ConfirmModal.IsVisible() ||
		v.getSeqModal.Visible || v.directGetModal.Visible || v.copyModal.Visible || v.gapsModal.Visible ||
		v.statsModal.Visible || v.clusterModal.Visible || v.stateModal.Visible
}

func (v *StreamsView) handleTab(gtx layout.Context, shift bool) {
	var tags []any
	if v.isModalVisible() {
		// When any modal is open, tab navigation is handled by the modal itself
		return
	}

	tags = []any{
		&v.AddBtn,
		&v.RefreshBtn,
	}

	isSelected := v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered)
	if isSelected {
		tags = append(tags, &v.DeleteBtn, &v.purgeBtn, &v.browseBtn, &v.getSeqBtn, &v.directGetBtn, &v.gapsBtn, &v.copyBtn, &v.stateBtn, &v.statsBtn, &v.clusterBtn)
		// Only add seal button if stream is not sealed
		if !v.filtered[v.SelectedIdx].IsSealed {
			tags = append(tags, &v.sealBtn)
		}
	}

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

func (v *StreamsView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(24), "JetStream Streams")
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

func (v *StreamsView) layoutActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
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
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.DeleteBtn, icons.ActionDelete, components.IconPositionStart, "Delete")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			cgtx.Constraints.Max.X = gtx.Dp(unit.Dp(100))
			btn := components.SecondaryButton(th, &v.purgeBtn, icons.ContentDeleteSweep, components.IconPositionStart, "Purge")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.browseBtn, icons.ActionVisibility, components.IconPositionStart, "Browse")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.getSeqBtn, icons.ActionSearch, components.IconPositionStart, "Get Seq")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.directGetBtn, icons.ActionLabel, components.IconPositionStart, "Direct Get")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.gapsBtn, icons.AlertWarning, components.IconPositionStart, "Gaps")
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
			btn := components.SecondaryButton(th, &v.stateBtn, icons.ActionInfo, components.IconPositionStart, "State")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.statsBtn, icons.EditorInsertChart, components.IconPositionStart, "Stats")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.clusterBtn, icons.DeviceStorage, components.IconPositionStart, "Cluster")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			// Disable if stream is already sealed
			if isSelected && v.filtered[v.SelectedIdx].IsSealed {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.sealBtn, icons.ActionCheckCircle, components.IconPositionStart, "Seal")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.SearchEditor.Layout(cgtx, th)
		}),
	)
}

func (v *StreamsView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.EmptyState || len(v.filtered) == 0 {
		return v.layoutEmptyState(gtx, th)
	}

	return v.Split.Layout(gtx, th,
		func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
					return v.layoutStreamsTable(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return v.Paginator.Layout(ccgtx, th)
				}),
			)
		},
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutStreamDetails(cgtx, th)
		},
	)
}

func (v *StreamsView) layoutEmptyState(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return components.EmptyState{
		Icon:    icons.DeviceStorage,
		Title:   "No Streams Found",
		Message: "Create a new JetStream stream to get started.",
	}.Layout(gtx, th)
}

func (v *StreamsView) promptStreamDelete(name string) {
	v.ConfirmModal.Title = "Delete Stream"
	v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to delete stream '%s'?", name)
	v.ConfirmModal.ReturnFocus = v.Table.FocusTag()
	v.ConfirmModal.SetOnClose(func() {
		v.RestoreListFocus = true
	})
	v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
		if option == "Confirm" {
			v.deleteStream(name)
		}
	})
	v.ConfirmModal.Show()
}

func (v *StreamsView) promptStreamPurge(name string) {
	v.ConfirmModal.Title = "Purge Stream"
	v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to purge stream '%s'? All messages will be deleted.", name)
	v.ConfirmModal.ReturnFocus = v.Table.FocusTag()
	v.ConfirmModal.SetOnClose(func() {
		v.RestoreListFocus = true
	})
	v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
		if option == "Confirm" {
			v.purgeStream(name)
		}
	})
	v.ConfirmModal.Show()
}

func (v *StreamsView) promptSealStream(name string) {
	v.ConfirmModal.Title = "Seal Stream"
	v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to seal stream '%s'? This will prevent any further modifications.", name)
	v.ConfirmModal.ReturnFocus = v.Table.FocusTag()
	v.ConfirmModal.SetOnClose(func() {
		v.RestoreListFocus = true
	})
	v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
		if option == "Confirm" {
			v.sealStream(name)
		}
	})
	v.ConfirmModal.Show()
}

func (v *StreamsView) layoutStreamsTable(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.Table.Rows = BuildTableRows(v.filtered, v.Paginator.CurrentPage, v.PerPage,
		func(stream *StreamInfo, idx int) components.TableRow {
			return components.TableRow{
				Values: []string{
					stream.Name,
					stream.Subjects,
					stream.Storage,
					utils.FormatNumber(stream.Messages),
					utils.FormatBytes(stream.Bytes),
					fmt.Sprintf("%d", stream.Consumers),
					stream.Created,
				},
			}
		}, v.SelectedIdx)

	return v.Table.Layout(gtx, th)
}

func (v *StreamsView) showCreateStreamModal() {
	// Clear all inputs
	v.nameInput.SetText("")
	v.subjectsInput.SetText("")
	v.descriptionInput.SetText("")
	v.storageTypeDropDown.SetSelected(0)
	v.retentionInput.SetSelected(0)
	v.maxMsgsInput.SetText("")
	v.maxBytesInput.SetText("")
	v.maxAgeInput.SetText("")
	v.replicasInput.SetText("")
	v.optionalSection.Expanded = false

	// Clear advanced fields
	v.mirrorNameInput.SetText("")
	v.mirrorFilterInput.SetText("")
	v.mirrorStartSeqInput.SetText("")
	v.sourcesInput.SetText("")
	v.republishSourceInput.SetText("")
	v.republishDestInput.SetText("")
	v.republishHeadersOnlyBool.Value = false
	v.transformSourceInput.SetText("")
	v.transformDestInput.SetText("")
	v.advancedSection.Expanded = false
	v.mirrorSection.Expanded = false

	v.createModal.Show()
}

func (v *StreamsView) handleCreateStream() bool {
	name := v.nameInput.GetText()
	subjects := strings.Fields(v.subjectsInput.GetText())
	mirrorName := v.mirrorNameInput.GetText()

	// For mirror streams, subjects are not required
	if mirrorName == "" && len(subjects) == 0 {
		if v.App != nil {
			v.App.ShowToast("Name and subjects are required (or configure a mirror)", components.ToastTypeError)
		}
		return false
	}

	if name == "" {
		if v.App != nil {
			v.App.ShowToast("Name is required", components.ToastTypeError)
		}
		return false
	}

	v.createStreamWithConfig(name, subjects)
	return true
}

func (v *StreamsView) createStreamWithConfig(name string, subjects []string) {
	if v.App == nil {
		return
	}
	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		config := jetstream.StreamConfig{
			Name: name,
		}

		// Only set subjects if not a mirror
		mirrorName := v.mirrorNameInput.GetText()
		if mirrorName == "" {
			config.Subjects = subjects
		}

		// Parse optional fields
		desc := v.descriptionInput.GetText()
		if desc != "" {
			config.Description = desc
		}

		storageType := "file"
		if selected := v.storageTypeDropDown.GetSelected(); selected != nil && selected.Value == "memory" {
			config.Storage = jetstream.MemoryStorage
			storageType = "memory"
		} else {
			config.Storage = jetstream.FileStorage
		}

		retention := "limits"
		if selected := v.retentionInput.GetSelected(); selected != nil {
			retention = selected.Value
		}
		switch retention {
		case "interest":
			config.Retention = jetstream.InterestPolicy
		case "workqueue":
			config.Retention = jetstream.WorkQueuePolicy
		default:
			config.Retention = jetstream.LimitsPolicy
		}

		if maxMsgs := v.maxMsgsInput.GetText(); maxMsgs != "" {
			if val, err := strconv.ParseInt(maxMsgs, 10, 64); err == nil {
				config.MaxMsgs = val
			}
		}

		if maxBytes := v.maxBytesInput.GetText(); maxBytes != "" {
			if val, err := strconv.ParseInt(maxBytes, 10, 64); err == nil {
				config.MaxBytes = val
			}
		}

		if maxAge := v.maxAgeInput.GetText(); maxAge != "" {
			if duration, err := time.ParseDuration(maxAge); err == nil {
				config.MaxAge = duration
			}
		}

		if replicas := v.replicasInput.GetText(); replicas != "" {
			if val, err := strconv.Atoi(replicas); err == nil {
				config.Replicas = val
			}
		}

		// Parse Mirror configuration
		if mirrorName != "" {
			mirror := &jetstream.StreamSource{
				Name: mirrorName,
			}
			if filter := v.mirrorFilterInput.GetText(); filter != "" {
				mirror.FilterSubject = filter
			}
			if startSeq := v.mirrorStartSeqInput.GetText(); startSeq != "" {
				if val, err := strconv.ParseUint(startSeq, 10, 64); err == nil {
					mirror.OptStartSeq = val
				}
			}
			config.Mirror = mirror
		}

		// Parse Sources configuration
		if sourcesStr := v.sourcesInput.GetText(); sourcesStr != "" {
			sourceNames := strings.Split(sourcesStr, ",")
			for _, srcName := range sourceNames {
				srcName = strings.TrimSpace(srcName)
				if srcName != "" {
					config.Sources = append(config.Sources, &jetstream.StreamSource{
						Name: srcName,
					})
				}
			}
		}

		// Parse Republish configuration
		if republishDest := v.republishDestInput.GetText(); republishDest != "" {
			republish := &jetstream.RePublish{
				Destination: republishDest,
			}
			if src := v.republishSourceInput.GetText(); src != "" {
				republish.Source = src
			}
			if v.republishHeadersOnlyBool.Value {
				republish.HeadersOnly = true
			}
			config.RePublish = republish
		}

		// Parse Subject Transform configuration
		if transformSrc := v.transformSourceInput.GetText(); transformSrc != "" {
			transformDest := v.transformDestInput.GetText()
			if transformDest != "" {
				config.SubjectTransform = &jetstream.SubjectTransformConfig{
					Source:      transformSrc,
					Destination: transformDest,
				}
			}
		}

		_, err := client.CreateStream(ctx, config)
		if err != nil {
			v.App.ShowToast("Failed to create stream: "+err.Error(), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", name).
				Strs("subjects", subjects).
				Str("description", config.Description).
				Int("replicas", config.Replicas).
				Str("storage", storageType).
				Str("retention", retention).
				Int64("max_msgs", config.MaxMsgs).
				Int64("max_bytes", config.MaxBytes).
				Dur("max_age", config.MaxAge).
				Bool("mirror", config.Mirror != nil).
				Str("mirror_name", mirrorName).
				Err(err).
				Msg("Stream creation failed")
			return
		}

		v.App.ShowToast("Stream created successfully", components.ToastTypeSuccess)
		log.Logger().Info().
			Str("stream", name).
			Strs("subjects", subjects).
			Str("description", config.Description).
			Int("replicas", config.Replicas).
			Str("storage", storageType).
			Str("retention", retention).
			Int64("max_msgs", config.MaxMsgs).
			Int64("max_bytes", config.MaxBytes).
			Dur("max_age", config.MaxAge).
			Bool("mirror", config.Mirror != nil).
			Str("mirror_name", mirrorName).
			Msg("Stream created")
		v.Refresh()
	}()
}

func (v *StreamsView) purgeStream(name string) {
	if v.App == nil {
		return
	}
	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Get stream info before purge for logging
		stream, err := client.GetStream(ctx, name)
		var prePurgeState uint64
		var prePurgeBytes uint64
		if err == nil {
			if info, err := stream.Info(ctx); err == nil {
				prePurgeState = info.State.Msgs
				prePurgeBytes = info.State.Bytes
			}
		}

		err = client.PurgeStream(ctx, name)
		if err != nil {
			v.App.ShowToast("Failed to purge stream: "+err.Error(), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", name).
				Uint64("messages", prePurgeState).
				Uint64("bytes", prePurgeBytes).
				Err(err).
				Msg("Stream purge failed")
			return
		}

		v.App.ShowToast("Stream purged successfully", components.ToastTypeSuccess)
		log.Logger().Info().
			Str("stream", name).
			Uint64("messages_purged", prePurgeState).
			Uint64("bytes_purged", prePurgeBytes).
			Msg("Stream purged")
		v.Refresh()
	}()
}

func (v *StreamsView) deleteStream(name string) {
	if v.App == nil {
		return
	}
	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Get stream info before delete for logging
		stream, err := client.GetStream(ctx, name)
		var preDeleteState uint64
		var preDeleteBytes uint64
		if err == nil {
			if info, err := stream.Info(ctx); err == nil {
				preDeleteState = info.State.Msgs
				preDeleteBytes = info.State.Bytes
			}
		}

		err = client.DeleteStream(ctx, name)
		if err != nil {
			v.App.ShowToast("Failed to delete stream: "+err.Error(), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", name).
				Uint64("messages", preDeleteState).
				Uint64("bytes", preDeleteBytes).
				Err(err).
				Msg("Stream deletion failed")
			return
		}

		v.App.ShowToast("Stream deleted successfully", components.ToastTypeSuccess)
		log.Logger().Info().
			Str("stream", name).
			Uint64("messages_deleted", preDeleteState).
			Uint64("bytes_deleted", preDeleteBytes).
			Msg("Stream deleted")
		v.SelectedIdx = -1
		v.Refresh()
	}()
}

func (v *StreamsView) layoutStreamDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Select a stream")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	stream := v.filtered[v.SelectedIdx]
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.Card{
					Title: stream.Name,
				}.Layout(ccgtx, th, func(cccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layout.Flex{Spacing: layout.SpaceBetween}.Layout(c4gtx,
								layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
									return components.StatCard{
										Title: "Messages",
										Value: utils.FormatNumber(stream.Messages),
									}.Layout(c5gtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
									return components.StatCard{
										Title: "Bytes",
										Value: utils.FormatBytes(stream.Bytes),
									}.Layout(c5gtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
									return components.StatCard{
										Title: "Consumers",
										Value: fmt.Sprintf("%d", stream.Consumers),
									}.Layout(c5gtx, th)
								}),
							)
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
							return layoutDetailRow(c4gtx, th, "Subjects", stream.Subjects)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Storage", stream.Storage)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Retention", stream.Retention)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Created", stream.Created)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return v.layoutStreamHealth(c4gtx, th)
						}),
					)
				})
			}),
		)
	})
}

// loadStreamMessages loads messages from a stream and opens the modal
func (v *StreamsView) loadStreamMessages(streamName string) {
	if v.App == nil || v.App.GetNatsClient() == nil {
		return
	}

	v.selectedStream = streamName
	v.streamMessages = []*jetstream.RawStreamMsg{}
	v.streamLastSeq = 0
	v.hasMoreMessages = true

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := v.App.GetNatsClient()
		messages, total, err := client.GetStreamMessages(ctx, streamName, 100, 0)
		if err != nil {
			v.App.ShowToast("Failed to load stream messages: "+err.Error(), components.ToastTypeError)
			return
		}

		v.streamMessages = messages
		v.streamTotalMessages = total

		// Determine if there are more messages to load
		if len(messages) > 0 {
			v.streamLastSeq = messages[len(messages)-1].Sequence
			v.hasMoreMessages = v.streamLastSeq > 1
		} else {
			v.hasMoreMessages = false
		}

		// Convert to viewer items
		viewerItems := v.convertStreamMessagesToViewerItems(messages)

		// Store the items and stream name for opening on the main thread
		v.pendingModalItems = viewerItems
		v.pendingModalTitle = fmt.Sprintf("Messages in %s", streamName)

		if v.App != nil && v.App.GetCurrentPageID() == navigator.StreamsPageId {
			v.App.Invalidate()
		}
	}()
}

// loadMoreStreamMessages loads more messages for infinite scroll
func (v *StreamsView) loadMoreStreamMessages() {
	if v.App == nil || v.App.GetNatsClient() == nil || !v.hasMoreMessages {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := v.App.GetNatsClient()
		// Load next batch starting from the sequence before the last loaded message
		startSeq := v.streamLastSeq - 1
		if startSeq < 1 {
			v.hasMoreMessages = false
			return
		}

		messages, _, err := client.GetStreamMessages(ctx, v.selectedStream, 100, startSeq)
		if err != nil {
			return
		}

		if len(messages) == 0 {
			v.hasMoreMessages = false
			return
		}

		// Append new messages to existing list
		v.streamMessages = append(v.streamMessages, messages...)

		// Update last sequence and check if there are more
		v.streamLastSeq = messages[len(messages)-1].Sequence
		v.hasMoreMessages = v.streamLastSeq > 1

		// Convert all messages to viewer items and update modal
		viewerItems := v.convertStreamMessagesToViewerItems(v.streamMessages)
		v.messagesModal.UpdateItems(viewerItems)
		v.messagesModal.SetHasMoreItems(v.hasMoreMessages)

		if v.App != nil {
			v.App.Invalidate()
		}
	}()
}

// convertStreamMessagesToViewerItems converts stream messages to MessageViewerItems
func (v *StreamsView) convertStreamMessagesToViewerItems(messages []*jetstream.RawStreamMsg) []components.MessageViewerItem {
	viewerItems := make([]components.MessageViewerItem, len(messages))
	for i, msg := range messages {
		subtitle := fmt.Sprintf("Seq: %d\nSize: %d bytes\nTime: %s", msg.Sequence, len(msg.Data), msg.Time.Format(time.DateTime))
		format := utils.DetectPayloadFormat(msg.Data)
		content := string(msg.Data)

		viewerItems[i] = components.MessageViewerItem{
			ID:       fmt.Sprintf("%d", msg.Sequence),
			Title:    msg.Subject,
			Subtitle: subtitle,
			Content:  content,
			Format:   format,
			Created:  msg.Time,
			Icon:     icons.ContentSend,
		}
	}
	return viewerItems
}

// promptMessageDelete shows a confirmation dialog for deleting a message
func (v *StreamsView) promptMessageDelete(seqStr string) {
	var seq uint64
	fmt.Sscanf(seqStr, "%d", &seq)

	v.ConfirmModal.Title = "Delete Message"
	v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to delete message with sequence %d from stream %s?", seq, v.selectedStream)
	v.ConfirmModal.ReturnFocus = v.messagesModal.FocusTag()
	v.ConfirmModal.SetOnClose(func() {
		// Only restore focus if the messages modal is still open
		if v.messagesModal.IsOpen {
			v.RestoreListFocus = true
		}
	})
	v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
		if option == "Confirm" {
			v.deleteStreamMessage(v.selectedStream, seq)
		}
	})
	v.ConfirmModal.Show()
}

// deleteStreamMessage deletes a specific message from a stream
func (v *StreamsView) deleteStreamMessage(streamName string, seq uint64) {
	if v.App == nil {
		return
	}
	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := client.DeleteStreamMessage(ctx, streamName, seq)
		if err != nil {
			v.App.ShowToast("Failed to delete message: "+err.Error(), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", streamName).
				Uint64("sequence", seq).
				Err(err).
				Msg("Stream message deletion failed")
			return
		}

		v.App.ShowToast(fmt.Sprintf("Message %d deleted successfully", seq), components.ToastTypeSuccess)
		log.Logger().Info().
			Str("stream", streamName).
			Uint64("sequence", seq).
			Msg("Stream message deleted")
		// Refresh the messages modal
		v.loadStreamMessages(streamName)
	}()
}

// checkStreamHealth checks the health of a single stream
func (v *StreamsView) checkStreamHealth(streamName string) {
	if v.App == nil {
		return
	}
	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		return
	}

	v.healthLoading = true
	v.selectedStreamHealth = nil
	v.App.Invalidate()

	go func() {
		defer func() { v.healthLoading = false }()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		stream, err := client.GetJetStream().Stream(ctx, streamName)
		if err != nil {
			v.selectedStreamHealth = &StreamHealth{
				Name:    streamName,
				Status:  "Critical",
				Details: fmt.Sprintf("Failed to get stream: %v", err),
			}
			if v.App != nil && v.App.GetCurrentPageID() == navigator.StreamsPageId {
				v.App.Invalidate()
			}
			return
		}

		info, err := stream.Info(ctx)
		if err != nil {
			v.selectedStreamHealth = &StreamHealth{
				Name:    streamName,
				Status:  "Critical",
				Details: fmt.Sprintf("Failed to get stream info: %v", err),
			}
			if v.App != nil && v.App.GetCurrentPageID() == navigator.StreamsPageId {
				v.App.Invalidate()
			}
			return
		}

		health := &StreamHealth{
			Name:         info.Config.Name,
			MessageCount: int64(info.State.Msgs),
			LastSeq:      info.State.LastSeq,
		}

		status := "Healthy"
		details := []string{}

		if info.State.Msgs == 0 {
			details = append(details, "Stream is empty")
		}

		if info.State.Consumers == 0 {
			details = append(details, "No consumers attached")
			if status == "Healthy" {
				status = "Warning"
			}
		}

		if info.Cluster != nil && len(info.Cluster.Replicas) > 0 {
			health.ClusterHealthy = true
			for _, replica := range info.Cluster.Replicas {
				if replica.Offline {
					health.ClusterHealthy = false
					status = "Critical"
					details = append(details, fmt.Sprintf("Replica %s is offline", replica.Name))
				}
			}
		}

		if len(info.Config.Sources) > 0 {
			health.SourcesHealthy = true
			details = append(details, fmt.Sprintf("Has %d sources", len(info.Config.Sources)))
		}

		if info.Config.Mirror != nil {
			health.MirrorHealthy = true
			details = append(details, "Has mirror configuration")
		}

		health.Status = status
		health.Details = strings.Join(details, "; ")
		v.selectedStreamHealth = health

		log.Logger().Info().
			Str("stream", streamName).
			Str("status", status).
			Int64("messages", health.MessageCount).
			Bool("cluster_healthy", health.ClusterHealthy).
			Bool("sources_healthy", health.SourcesHealthy).
			Bool("mirror_healthy", health.MirrorHealthy).
			Msg("Stream health check completed")

		if v.App != nil && v.App.GetCurrentPageID() == navigator.StreamsPageId {
			v.App.Invalidate()
		}
	}()
}

// showGetBySequenceModal shows the modal for getting a message by sequence
func (v *StreamsView) showGetBySequenceModal() {
	v.seqInput.SetValue(0)
	v.getSeqModal.Show()
}

// handleGetBySequence handles the Get By Sequence action
func (v *StreamsView) handleGetBySequence() bool {
	seq := v.seqInput.GetValue()
	if seq <= 0 {
		if v.App != nil {
			v.App.ShowToast("Please enter a valid sequence number", components.ToastTypeError)
		}
		return false
	}

	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return false
	}

	streamName := v.filtered[v.SelectedIdx].Name

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := v.App.GetNatsClient()
		msg, err := client.GetStreamMessageBySequence(ctx, streamName, uint64(seq))
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to get message: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", streamName).
				Uint64("sequence", uint64(seq)).
				Err(err).
				Msg("Get message by sequence failed")
			return
		}

		// Log successful get
		log.Logger().Info().
			Str("stream", streamName).
			Uint64("sequence", uint64(seq)).
			Str("subject", msg.Subject).
			Int("payload_size", len(msg.Data)).
			Msg("Message retrieved by sequence")

		// Create a single-item viewer for this message
		viewerItems := v.convertStreamMessagesToViewerItems([]*jetstream.RawStreamMsg{msg})
		v.pendingModalItems = viewerItems
		v.pendingModalTitle = fmt.Sprintf("Message %d in %s", seq, streamName)

		if v.App != nil && v.App.GetCurrentPageID() == navigator.StreamsPageId {
			v.App.Invalidate()
		}
	}()

	return true
}

// handleDirectGetBySubject handles the Direct Get by Subject action
func (v *StreamsView) handleDirectGetBySubject() bool {
	subject := v.directGetInput.GetText()
	if subject == "" {
		if v.App != nil {
			v.App.ShowToast("Please enter a subject pattern", components.ToastTypeError)
		}
		return false
	}

	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return false
	}

	streamName := v.filtered[v.SelectedIdx].Name

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := v.App.GetNatsClient()
		msg, err := client.GetStreamMessageBySubject(ctx, streamName, subject)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to get message: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", streamName).
				Str("subject", subject).
				Err(err).
				Msg("Get message by subject failed")
			return
		}

		// Log successful get
		log.Logger().Info().
			Str("stream", streamName).
			Str("subject", subject).
			Uint64("sequence", msg.Sequence).
			Int("payload_size", len(msg.Data)).
			Msg("Message retrieved by subject")

		// Create a single-item viewer for this message
		viewerItems := v.convertStreamMessagesToViewerItems([]*jetstream.RawStreamMsg{msg})
		v.pendingModalItems = viewerItems
		v.pendingModalTitle = fmt.Sprintf("Last message for '%s' in %s", subject, streamName)

		if v.App != nil && v.App.GetCurrentPageID() == navigator.StreamsPageId {
			v.App.Invalidate()
		}
	}()

	return true
}

// handleCopyStream handles the copy stream action
func (v *StreamsView) handleCopyStream() bool {
	newName := v.copyNameInput.GetText()
	if newName == "" {
		if v.App != nil {
			v.App.ShowToast("Please enter a new stream name", components.ToastTypeError)
		}
		return false
	}

	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return false
	}

	sourceStreamName := v.filtered[v.SelectedIdx].Name

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client := v.App.GetNatsClient()
		_, err := client.CopyStream(ctx, sourceStreamName, newName)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to copy stream: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("source_stream", sourceStreamName).
				Str("new_stream", newName).
				Err(err).
				Msg("Stream copy failed")
			return
		}

		v.App.ShowToast(fmt.Sprintf("Stream copied: %s -> %s", sourceStreamName, newName), components.ToastTypeSuccess)
		log.Logger().Info().
			Str("source_stream", sourceStreamName).
			Str("new_stream", newName).
			Msg("Stream copied")
		v.Refresh()
	}()

	return true
}

// sealStream seals the selected stream
func (v *StreamsView) sealStream(streamName string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := v.App.GetNatsClient()
		err := client.SealStream(ctx, streamName)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to seal stream: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", streamName).
				Err(err).
				Msg("Stream seal failed")
			return
		}

		v.App.ShowToast(fmt.Sprintf("Stream %s sealed successfully", streamName), components.ToastTypeSuccess)
		log.Logger().Info().
			Str("stream", streamName).
			Msg("Stream sealed")
		v.Refresh()
	}()
}

// detectMessageGaps detects gaps in message sequences for a stream
func (v *StreamsView) detectMessageGaps(streamName string) {
	if v.App == nil {
		return
	}

	client := v.App.GetNatsClient()
	if client == nil || !client.IsConnected() {
		v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
		return
	}

	v.gapsLoading = true
	v.gapsModal.Show()
	v.gapsResults = nil

	go func() {
		defer func() { v.gapsLoading = false }()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		gaps, err := client.DetectMessageGaps(ctx, streamName)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to detect gaps: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("stream", streamName).
				Err(err).
				Msg("Message gap detection failed")
			return
		}

		v.gapsResults = gaps

		if len(gaps) == 0 {
			v.App.ShowToast(fmt.Sprintf("No gaps found in stream %s", streamName), components.ToastTypeSuccess)
			log.Logger().Info().
				Str("stream", streamName).
				Int("gaps_found", 0).
				Msg("Message gap detection completed - no gaps")
		} else {
			v.App.ShowToast(fmt.Sprintf("Found %d gap(s) in stream %s", len(gaps), streamName), components.ToastTypeWarning)
			log.Logger().Warn().
				Str("stream", streamName).
				Int("gaps_found", len(gaps)).
				Msg("Message gap detection completed - gaps found")
		}

		if v.App != nil && v.App.GetCurrentPageID() == navigator.StreamsPageId {
			v.App.Invalidate()
		}
	}()
}

// showStreamState fetches and displays the detailed state of a stream
func (v *StreamsView) showStreamState(streamName string) {
	if v.App == nil {
		return
	}

	v.stateLoading = true
	v.stateModal.Show()
	v.stateResults = nil

	go func() {
		defer func() { v.stateLoading = false }()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := v.App.GetNatsClient()
		if client == nil || !client.IsConnected() {
			v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
			return
		}

		stream, err := client.GetJetStream().Stream(ctx, streamName)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to get stream: %v", err), components.ToastTypeError)
			return
		}

		info, err := stream.Info(ctx)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to get stream info: %v", err), components.ToastTypeError)
			return
		}

		v.stateResults = &info.State

		log.Logger().Info().
			Str("stream", streamName).
			Uint64("messages", info.State.Msgs).
			Uint64("bytes", info.State.Bytes).
			Uint64("first_seq", info.State.FirstSeq).
			Uint64("last_seq", info.State.LastSeq).
			Msg("Stream state viewed")

		if v.App != nil && v.App.GetCurrentPageID() == navigator.StreamsPageId {
			v.App.Invalidate()
		}
	}()
}

// showStreamStats loads and displays stream statistics
func (v *StreamsView) showStreamStats(streamName string) {
	if v.App == nil {
		return
	}

	v.statsModal.Show()
	v.selectedStreamForStats = streamName
	v.streamStats = StreamStatsData{}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		client := v.App.GetNatsClient()
		if client == nil || !client.IsConnected() {
			v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
			return
		}

		// Get stream info
		stream, err := client.GetJetStream().Stream(ctx, streamName)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to get stream: %v", err), components.ToastTypeError)
			return
		}

		info, err := stream.Info(ctx)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to get stream info: %v", err), components.ToastTypeError)
			return
		}

		// Populate stream stats
		v.streamStats = StreamStatsData{
			TotalMessages: info.State.Msgs,
			TotalBytes:    info.State.Bytes,
			FirstSeq:      info.State.FirstSeq,
			LastSeq:       info.State.LastSeq,
			NumSubjects:   info.State.NumSubjects,
			NumConsumers:  info.State.Consumers,
		}

		// Get consumer statistics
		if info.State.Consumers > 0 {
			iterator := stream.ListConsumers(ctx)
			for c := range iterator.Info() {
				if c != nil {
					v.streamStats.ConsumerStats = append(v.streamStats.ConsumerStats, ConsumerStat{
						Name:       c.Name,
						Pending:    int64(c.NumPending),
						Delivered:  int64(c.Delivered.Consumer),
						AckPending: int64(c.NumAckPending),
					})
				}
			}
			if iterator.Err() != nil {
				v.App.ShowToast(fmt.Sprintf("Error loading consumers: %v", iterator.Err()), components.ToastTypeWarning)
			}
		}

		log.Logger().Info().
			Str("stream", streamName).
			Uint64("total_messages", v.streamStats.TotalMessages).
			Uint64("total_bytes", v.streamStats.TotalBytes).
			Int("num_consumers", len(v.streamStats.ConsumerStats)).
			Msg("Stream statistics viewed")

		if v.App != nil && v.App.GetCurrentPageID() == navigator.StreamsPageId {
			v.App.Invalidate()
		}
	}()
}

// showStreamCluster loads and displays stream cluster information
func (v *StreamsView) showStreamCluster(streamName string) {
	if v.App == nil {
		return
	}

	v.clusterLoading = true
	v.clusterModal.Show()
	v.selectedStreamForCluster = streamName
	v.clusterInfo = nil

	go func() {
		defer func() { v.clusterLoading = false }()

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		client := v.App.GetNatsClient()
		if client == nil || !client.IsConnected() {
			v.App.ShowToast("Not connected to NATS", components.ToastTypeError)
			return
		}

		// Get stream info
		stream, err := client.GetJetStream().Stream(ctx, streamName)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to get stream: %v", err), components.ToastTypeError)
			return
		}

		info, err := stream.Info(ctx)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to get stream info: %v", err), components.ToastTypeError)
			return
		}

		if info.Cluster == nil {
			v.App.ShowToast("Stream is not clustered", components.ToastTypeWarning)
			v.clusterInfo = nil
			log.Logger().Info().
				Str("stream", streamName).
				Bool("clustered", false).
				Msg("Stream cluster info viewed")
		} else {
			v.clusterInfo = info.Cluster
			log.Logger().Info().
				Str("stream", streamName).
				Bool("clustered", true).
				Str("leader", info.Cluster.Leader).
				Int("replicas", len(info.Cluster.Replicas)).
				Msg("Stream cluster info viewed")
		}

		if v.App != nil && v.App.GetCurrentPageID() == navigator.StreamsPageId {
			v.App.Invalidate()
		}
	}()
}

// layoutGapsContent renders the content for the gaps modal
func (v *StreamsView) layoutGapsContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.gapsLoading {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Scanning for gaps...")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	if len(v.gapsResults) == 0 {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "No message gaps detected")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), fmt.Sprintf("Found %d gap(s):", len(v.gapsResults)))
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.layoutGapsList(cgtx, th)
		}),
	)
}

// layoutGapsList renders the list of gaps
func (v *StreamsView) layoutGapsList(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(v.gapsResults)*2)
	for i, gap := range v.gapsResults {
		idx := i
		children = append(children,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.layoutGapRow(cgtx, th, idx+1, gap)
			}),
		)
		if i < len(v.gapsResults)-1 {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			)
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// layoutGapRow renders a single gap row
func (v *StreamsView) layoutGapRow(gtx layout.Context, th *theme.Theme, num int, gap nats.MessageGap) layout.Dimensions {
	return components.Card{
		Title: fmt.Sprintf("Gap #%d", num),
	}.Layout(gtx, th, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Start Seq", fmt.Sprintf("%d", gap.StartSeq))
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "End Seq", fmt.Sprintf("%d", gap.EndSeq))
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layoutDetailRow(ccgtx, th, "Missing Count", fmt.Sprintf("%d", gap.Count))
			}),
		)
	})
}

// layoutStatsContent renders the content for the stats modal
func (v *StreamsView) layoutStatsContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Total Messages", utils.FormatNumber(int64(v.streamStats.TotalMessages)))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Total Bytes", utils.FormatBytes(int64(v.streamStats.TotalBytes)))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "First Sequence", fmt.Sprintf("%d", v.streamStats.FirstSeq))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Last Sequence", fmt.Sprintf("%d", v.streamStats.LastSeq))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Num Subjects", fmt.Sprintf("%d", v.streamStats.NumSubjects))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Num Consumers", fmt.Sprintf("%d", v.streamStats.NumConsumers))
		}),
	)
}

// layoutClusterContent renders the content for the cluster modal
func (v *StreamsView) layoutClusterContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.clusterLoading {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Loading cluster info...")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	if v.clusterInfo == nil {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "No cluster information available")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	cluster := v.clusterInfo
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Cluster Name", cluster.Name)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Raft Group", cluster.RaftGroup)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			leader := cluster.Leader
			if leader == "" {
				leader = "N/A"
			}
			return layoutDetailRow(cgtx, th, "Leader", leader)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if cluster.LeaderSince != nil {
				return layoutDetailRow(cgtx, th, "Leader Since", cluster.LeaderSince.Format("2006-01-02 15:04:05"))
			}
			return layoutDetailRow(cgtx, th, "Leader Since", "N/A")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Replicas")
			lbl.Font.Weight = font.Bold
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.layoutReplicasList(cgtx, th, cluster.Replicas)
		}),
	)
}

// layoutReplicasList renders the list of replicas
func (v *StreamsView) layoutReplicasList(gtx layout.Context, th *theme.Theme, replicas []*jetstream.PeerInfo) layout.Dimensions {
	if len(replicas) == 0 {
		lbl := material.Label(th.Material(), unit.Sp(13), "No replicas")
		lbl.Color = th.TextColor
		return lbl.Layout(gtx)
	}

	list := &widget.List{
		List: layout.List{
			Axis: layout.Vertical,
		},
	}

	listStyle := material.List(th.Material(), list)
	return listStyle.Layout(gtx, len(replicas), func(cgtx layout.Context, index int) layout.Dimensions {
		peer := replicas[index]
		status := "Current"
		if peer.Offline {
			status = "Offline"
		} else if !peer.Current {
			status = "Behind"
		}

		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						lbl := material.Label(th.Material(), unit.Sp(13), peer.Name)
						lbl.Font.Weight = font.Bold
						lbl.Color = th.TextColor
						return lbl.Layout(cccgtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						lbl := material.Label(th.Material(), unit.Sp(13), status)
						if peer.Offline {
							lbl.Color = th.ErrorColor
						} else if peer.Current {
							lbl.Color = th.Palette.ContrastFg
						} else {
							lbl.Color = th.WarningColor
						}
						return lbl.Layout(cccgtx)
					}),
				)
			}),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(ccgtx,
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						lbl := material.Label(th.Material(), unit.Sp(12), fmt.Sprintf("Active: %s", peer.Active))
						lbl.Color = th.SecondaryTextColor
						return lbl.Layout(cccgtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
						lbl := material.Label(th.Material(), unit.Sp(12), fmt.Sprintf("Lag: %d", peer.Lag))
						lbl.Color = th.SecondaryTextColor
						return lbl.Layout(cccgtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		)
	})
}

// layoutStateContent renders the content for the state modal
func (v *StreamsView) layoutStateContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.stateLoading {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Loading stream state...")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	if v.stateResults == nil {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Failed to load stream state")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	state := v.stateResults
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Messages", utils.FormatNumber(int64(state.Msgs)))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Bytes", utils.FormatBytes(int64(state.Bytes)))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "First Sequence", fmt.Sprintf("%d", state.FirstSeq))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Last Sequence", fmt.Sprintf("%d", state.LastSeq))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "First Timestamp", state.FirstTime.Format("2006-01-02 15:04:05"))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Last Timestamp", state.LastTime.Format("2006-01-02 15:04:05"))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Num Subjects", fmt.Sprintf("%d", state.NumSubjects))
		}),
	)
}

func (v *StreamsView) layoutStreamHealth(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.healthLoading {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(13), "Checking health...")
				lbl.Color = th.SecondaryTextColor
				return lbl.Layout(cgtx)
			}),
		)
	}

	if v.selectedStreamHealth == nil {
		return layout.Dimensions{}
	}

	health := v.selectedStreamHealth
	var statusType components.StatusPillType
	switch health.Status {
	case "Healthy":
		statusType = components.StatusPillSuccess
	case "Warning":
		statusType = components.StatusPillWarning
	case "Critical":
		statusType = components.StatusPillError
	default:
		statusType = components.StatusPillNeutral
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					lbl := material.Label(th.Material(), unit.Sp(13), "Health:")
					lbl.Font.Weight = font.Bold
					lbl.Color = th.TextColor
					return lbl.Layout(ccgtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return components.StatusPill{
						Text: health.Status,
						Type: statusType,
					}.Layout(ccgtx, th)
				}),
			)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if health.Details == "" {
				return layout.Dimensions{}
			}
			lbl := material.Label(th.Material(), unit.Sp(12), health.Details)
			lbl.Color = th.SecondaryTextColor
			return lbl.Layout(cgtx)
		}),
	)
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *StreamsView) HandleShortcuts(gtx layout.Context) bool {
	// Check if any modal is visible first - view shortcuts disabled when modal is open
	if v.isModalVisible() {
		return false
	}

	// View-specific shortcuts - process only ONE event per frame
	ev, ok := gtx.Event(
		key.Filter{Name: key.Name("N"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("R"), Optional: key.ModShortcut},
		key.Filter{Name: key.Name("B"), Optional: key.ModShortcut},
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
		case ke.Name == key.Name("B") && ke.Modifiers.Contain(key.ModShortcut):
			if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
				v.browseBtn.Click()
				return true
			}
		case ke.Name == key.NameDeleteForward || ke.Name == key.NameDeleteBackward:
			if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
				v.DeleteBtn.Click()
				return true
			}
		case ke.Name == key.NameReturn || ke.Name == key.NameEnter:
			if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
				v.browseBtn.Click()
				return true
			}
		}
	}
	return false
}

// GetShortcutsHelp returns help text for this view's shortcuts
func (v *StreamsView) GetShortcutsHelp() []shortcuts.Shortcut {
	return []shortcuts.Shortcut{
		shortcuts.Create(func() {}),
		shortcuts.Refresh(func() {}),
		shortcuts.Delete(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.Browse(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.CopyName(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.CopyRow(func() bool { return v.SelectedIdx >= 0 }, func() {}),
	}
}
