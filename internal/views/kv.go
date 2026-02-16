package views

import (
	"context"
	"fmt"
	"image"
	"sort"
	"strconv"
	"strings"
	"time"

	"gioui.org/io/event"

	"github.com/thedataflows/nats-desktop/internal/models"
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

	"github.com/nats-io/nats.go/jetstream"
	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
	"github.com/thedataflows/nats-desktop/internal/utils"
)

type KVView struct {
	*BaseView

	stores   []*KVStoreInfo
	filtered []*KVStoreInfo

	// Extra buttons not in BaseView
	browseBtn widget.Clickable
	watchBtn  widget.Clickable
	purgeBtn  widget.Clickable
	addKeyBtn widget.Clickable

	// Create Bucket Modal
	createModal *components.FormModal

	// Add/Edit Key Modal
	keyModal           *components.FormModal
	keyNameInput       *components.InputField
	keyValueEditor     *components.CodeEditor
	keyRevisionInput   *components.InputField
	keyEditMode        bool // true = edit, false = create
	selectedKeyForEdit string
	keyOriginalValue   string // store original value for comparison when editing

	// Mandatory fields
	bucketNameInput *components.InputField

	// Optional fields (in expandable section)
	optionalSection     *components.ExpandableSection
	descriptionInput    *components.InputField
	historyInput        *components.InputField
	maxValueSizeInput   *components.InputField
	maxBytesInput       *components.InputField
	ttlInput            *components.InputField
	storageTypeDropDown *components.DropDown

	// Additional filters
	filterInput *components.InputField

	// Items Modal - using reusable component
	itemsModal        *components.MessageViewerModal
	selectedBucket    string
	bucketItems       []models.KVItem
	pendingModalItems []components.MessageViewerItem
	pendingModalTitle string

	// Infinite scroll state (for future pagination support)
	hasMoreItems  bool
	isLoadingMore bool

	next, prev any
}

type KVStoreInfo struct {
	Name     string
	Bucket   string
	Keys     int64
	Values   int64
	History  bool
	Replicas int
	MaxAge   string
	Storage  string
	Status   string
	Created  string
}

func NewKVView(th *theme.Theme) *KVView {
	v := &KVView{
		BaseView: NewBaseView(
			[]string{"Name", "Keys", "History", "Storage", "Status"},
			15,
		),
		stores:            []*KVStoreInfo{},
		filtered:          []*KVStoreInfo{},
		bucketItems:       []models.KVItem{},
		itemsModal:        components.NewMessageViewerModal(th),
		createModal:       components.NewFormModal("Create Bucket"),
		bucketNameInput:   components.NewLabeledInputFieldWithPosition("Bucket name", "", components.LabelPositionTop),
		optionalSection:   components.NewExpandableSection("Advanced Options"),
		descriptionInput:  components.NewLabeledInputFieldWithPosition("Description", "", components.LabelPositionTop),
		historyInput:      components.NewLabeledInputFieldWithPosition("History", "", components.LabelPositionTop),
		maxValueSizeInput: components.NewLabeledInputFieldWithPosition("Max value size", "", components.LabelPositionTop),
		maxBytesInput:     components.NewLabeledInputFieldWithPosition("Max bucket size", "", components.LabelPositionTop),
		ttlInput:          components.NewLabeledInputFieldWithPosition("TTL", "", components.LabelPositionTop),
		storageTypeDropDown: components.NewLabeledDropDown("Storage type",
			components.NewDropDownOption("Memory").WithValue("memory").DefaultSelected(),
			components.NewDropDownOption("File").WithValue("file"),
		),
		// Key modal
		keyModal:         components.NewFormModal("Add Key"),
		keyNameInput:     components.NewLabeledInputFieldWithPosition("Key name", "", components.LabelPositionTop),
		keyValueEditor:   components.NewCodeEditor("", components.CodeLanguageJSON, th),
		keyRevisionInput: components.NewLabeledInputFieldWithPosition("Revision", "0 for latest", components.LabelPositionTop),
	}

	// Make key value editor writable for add/edit operations
	v.keyValueEditor.SetReadOnly(false)

	// Override default split ratio for KV view
	v.Split.Resize.Ratio = 0.6
	v.filterInput = components.NewInputField("Search buckets...")
	v.filterInput.SetIcon(icons.ActionSearch, components.IconPositionStart)

	// Set up key modal
	// Return focus to items modal list so selected item gets focus after closing
	v.keyModal.ReturnFocus = v.itemsModal.FocusTag()
	v.keyModal.MaxHeight = unit.Dp(500)
	v.keyModal.MaxWidth = unit.Dp(450)

	// Set up tab handler for the code editor to navigate within the modal
	v.keyValueEditor.SetOnTab(func(gtx layout.Context, shift bool) {
		// Trigger FormModal's tab handling
		v.keyModal.HandleTabNavigation(gtx, shift)
	})
	v.keyModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				if v.keyEditMode {
					lbl := material.Label(th.Material(), unit.Sp(13), fmt.Sprintf("Editing key: %s", v.selectedKeyForEdit))
					lbl.Color = th.TextColor
					return lbl.Layout(cgtx)
				}
				return v.keyNameInput.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				lbl := material.Label(th.Material(), unit.Sp(13), "Value")
				lbl.Color = th.TextColor
				return lbl.Layout(cgtx)
			}),
			layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
				// Limit code editor height to ensure buttons are visible
				cgtx.Constraints.Max.Y = cgtx.Dp(unit.Dp(250))
				return v.keyValueEditor.Layout(cgtx, th)
			}),
		)
	}
	v.keyModal.OnSave = func() bool {
		return v.handleSaveKey()
	}
	v.keyModal.OnClose = func() {
		v.RestoreListFocus = true
	}

	// Set up items modal actions
	v.itemsModal.SetActions(
		func(item components.MessageViewerItem) {
			v.promptKeyDelete(item.ID)
		},
		func(item components.MessageViewerItem) {
			v.promptKeyPurge(item.ID)
		},
		func(item components.MessageViewerItem) {
			// Edit key action
			if !item.Deleted {
				v.showEditKeyModal(item.ID)
			}
		},
	)

	// Set up double-click to edit
	v.itemsModal.SetOnDoubleClick(func(item components.MessageViewerItem) {
		if !item.Deleted {
			v.showEditKeyModal(item.ID)
		}
	})

	// Set up Add Key button in items modal header
	v.itemsModal.SetAddAction(func() {
		v.showAddKeyModal()
	})

	// Enable "Show Deleted" checkbox for KV view
	v.itemsModal.SetShowDeletedCheckbox(true)

	// Set up content loader for items modal
	v.itemsModal.SetOnLoadContent(func(item components.MessageViewerItem) string {
		return v.loadKeyValue(item.ID)
	})

	// Set up infinite scroll loading (currently loads all at once, but supports the interface)
	v.itemsModal.SetOnLoadMore(func() {
		// KV loads all keys at once, so no pagination needed
		// This is a no-op for now but keeps the interface consistent
	})
	v.itemsModal.SetHasMoreItems(false)

	// Set up confirmation modal visibility checker (also checks keyModal)
	v.itemsModal.IsConfirmationModalVisible = func() bool {
		return v.ConfirmModal.IsVisible() || v.keyModal.Visible
	}

	// Set up invalidation callback
	v.itemsModal.SetOnInvalidate(func() {
		if v.App != nil {
			v.App.Invalidate()
		}
	})

	// Set up close callback to return focus to parent table
	v.itemsModal.SetOnClose(func() {
		v.RestoreListFocus = true
	})

	// Set up create bucket modal with custom content
	v.createModal.ReturnFocus = v.Table.FocusTag()
	v.createModal.MaxWidth = unit.Dp(500)
	v.createModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.bucketNameInput.Layout(cgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return v.optionalSection.Layout(cgtx, th, func(ccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(ccgtx,
						// Row 1: Description | History
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.descriptionInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.historyInput.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						// Row 2: Max Value Size | Max Bucket Size
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.maxValueSizeInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.maxBytesInput.Layout(ccccgtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						// Row 3: TTL | Storage Type
						layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(cccgtx,
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									return v.ttlInput.Layout(ccccgtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(0.47, func(ccccgtx layout.Context) layout.Dimensions {
									// Ensure dropdown has proper minimum width
									ccccgtx.Constraints.Min.X = ccccgtx.Dp(unit.Dp(120))
									return v.storageTypeDropDown.Layout(ccccgtx, th)
								}),
							)
						}),
					)
				})
			}),
		)
	}
	// Set up TAB navigation order for the modal - using dynamic function to handle collapsed sections
	v.createModal.CustomFocusTagsFunc = func() []event.Tag {
		tags := []event.Tag{
			v.bucketNameInput.FocusTag(),
			v.optionalSection.FocusTag(),
		}
		if v.optionalSection.Expanded {
			tags = append(tags,
				v.descriptionInput.FocusTag(),
				v.historyInput.FocusTag(),
				v.maxValueSizeInput.FocusTag(),
				v.maxBytesInput.FocusTag(),
				v.ttlInput.FocusTag(),
				v.storageTypeDropDown.FocusTag(),
			)
		}
		return tags
	}
	v.createModal.OnSave = func() bool {
		return v.handleCreateBucket()
	}
	v.createModal.OnClose = func() {
		v.RestoreListFocus = true
	}

	return v
}

func (v *KVView) SetApp(app App) {
	v.App = app
}

func (v *KVView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *KVView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.KVPageId,
		Title: "Key-Value",
		Icon:  icons.ActionLabel,
	}
}

func (v *KVView) OnEnter() {
	v.Refresh()
}

func (v *KVView) FirstFocusTag() any {
	return v.filterInput.FocusTag()
}

func (v *KVView) LastFocusTag() any {
	return v.Paginator.NextClick
}

func (v *KVView) showCreateBucketModal() {
	// Clear all inputs
	v.bucketNameInput.SetText("")
	v.descriptionInput.SetText("")
	v.historyInput.SetText("")
	v.maxValueSizeInput.SetText("")
	v.maxBytesInput.SetText("")
	v.ttlInput.SetText("")
	v.storageTypeDropDown.SetSelectedByValue("memory")
	v.optionalSection.Expanded = false

	v.createModal.Show()
}

func (v *KVView) handleCreateBucket() bool {
	bucket := v.bucketNameInput.GetText()

	if bucket == "" {
		if v.App != nil {
			v.App.ShowToast("Bucket name is required", components.ToastTypeError)
		}
		return false
	}

	v.createBucketWithConfig(bucket)
	return true
}

func (v *KVView) createBucketWithConfig(bucket string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		config := jetstream.KeyValueConfig{
			Bucket: bucket,
		}

		// Parse optional fields
		desc := v.descriptionInput.GetText()
		if desc != "" {
			config.Description = desc
		}

		if history := v.historyInput.GetText(); history != "" {
			if val, err := strconv.ParseUint(history, 10, 8); err == nil {
				if val > 0 && val <= 64 {
					config.History = uint8(val)
				}
			}
		}

		if maxValSize := v.maxValueSizeInput.GetText(); maxValSize != "" {
			if val, err := strconv.ParseInt(maxValSize, 10, 32); err == nil {
				config.MaxValueSize = int32(val)
			}
		}

		if maxBytes := v.maxBytesInput.GetText(); maxBytes != "" {
			if val, err := strconv.ParseInt(maxBytes, 10, 64); err == nil {
				config.MaxBytes = val
			}
		}

		if ttl := v.ttlInput.GetText(); ttl != "" {
			if duration, err := time.ParseDuration(ttl); err == nil {
				config.TTL = duration
			}
		}

		selectedOption := v.storageTypeDropDown.GetSelected()
		if selectedOption != nil && selectedOption.GetValue() == "file" {
			config.Storage = jetstream.FileStorage
		} else {
			config.Storage = jetstream.MemoryStorage
		}

		storageType := "memory"
		if config.Storage == jetstream.FileStorage {
			storageType = "file"
		}

		_, err := v.App.NATS().CreateKeyValue(ctx, config)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to create bucket: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("bucket", bucket).
				Str("description", config.Description).
				Uint8("history", config.History).
				Int32("max_value_size", config.MaxValueSize).
				Int64("max_bytes", config.MaxBytes).
				Dur("ttl", config.TTL).
				Str("storage", storageType).
				Err(err).
				Msg("KV bucket creation failed")
		} else {
			v.App.ShowToast("Bucket created successfully", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("bucket", bucket).
				Str("description", config.Description).
				Uint8("history", config.History).
				Int32("max_value_size", config.MaxValueSize).
				Int64("max_bytes", config.MaxBytes).
				Dur("ttl", config.TTL).
				Str("storage", storageType).
				Msg("KV bucket created")
			v.Refresh()
		}
	}()
}

func (v *KVView) createBucket(bucket string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := v.App.NATS().CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket: bucket,
		})
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to create bucket: %v", err), components.ToastTypeError)
		} else {
			v.App.ShowToast("Bucket created successfully", components.ToastTypeSuccess)
			v.Refresh()
		}
		if v.App.GetCurrentPageID() == navigator.KVPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *KVView) deleteBucket(bucket string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := v.App.NATS().DeleteKeyValue(ctx, bucket)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to delete bucket: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("bucket", bucket).
				Err(err).
				Msg("Failed to delete KV bucket")
		} else {
			v.App.ShowToast("Bucket deleted", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("bucket", bucket).
				Msg("KV bucket deleted")
			v.SelectedIdx = -1
			v.Refresh()
		}
		if v.App.GetCurrentPageID() == navigator.KVPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *KVView) purgeBucket(bucket string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := v.App.NATS().PurgeKeyValue(ctx, bucket)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to purge bucket: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("bucket", bucket).
				Err(err).
				Msg("Failed to purge KV bucket")
		} else {
			v.App.ShowToast("Bucket purged", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("bucket", bucket).
				Msg("KV bucket purged")
			v.Refresh()
		}
		if v.App.GetCurrentPageID() == navigator.KVPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *KVView) deleteKey(bucket, k string) {
	if v.App == nil || v.App.NATS() == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := v.App.NATS().DeleteKeyValueKey(ctx, bucket, k)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to delete key: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("bucket", bucket).
				Str("key", k).
				Err(err).
				Msg("KV key deletion failed")
		} else {
			v.App.ShowToast("Key marked as deleted", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("bucket", bucket).
				Str("key", k).
				Msg("KV key deleted")
			v.loadBucketKeys(bucket)
			v.Refresh()
		}
		if v.App.GetCurrentPageID() == navigator.KVPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *KVView) purgeKey(bucket, k string) {
	if v.App == nil || v.App.NATS() == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := v.App.NATS().PurgeKeyValueKey(ctx, bucket, k)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to purge key: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("bucket", bucket).
				Str("key", k).
				Err(err).
				Msg("KV key purge failed")
		} else {
			v.App.ShowToast("Key purged", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("bucket", bucket).
				Str("key", k).
				Msg("KV key purged")
			v.loadBucketKeys(bucket)
			v.Refresh()
		}
		if v.App.GetCurrentPageID() == navigator.KVPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *KVView) showAddKeyModal() {
	v.keyEditMode = false
	v.selectedKeyForEdit = ""
	v.keyNameInput.SetText("")
	v.keyValueEditor.SetCode("")
	// Set focus tags for add mode: key name input and value editor
	v.keyModal.CustomFocusTags = []event.Tag{
		v.keyNameInput.FocusTag(),
		v.keyValueEditor.FocusTag(),
	}
	v.keyModal.Title = "Add Key"
	v.keyModal.Show()
	v.keyValueEditor.RequestFocus()
}

func (v *KVView) showEditKeyModal(key string) {
	v.keyEditMode = true
	v.selectedKeyForEdit = key
	// Set focus tags for edit mode: only value editor (key name shows as label)
	v.keyModal.CustomFocusTags = []event.Tag{
		v.keyValueEditor.FocusTag(),
	}

	// Load current value
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		value, err := v.App.NATS().GetKeyValue(ctx, v.selectedBucket, key)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to load key value: %v", err), components.ToastTypeError)
			return
		}

		v.keyOriginalValue = string(value)
		v.keyValueEditor.SetCode(v.keyOriginalValue)
		v.keyModal.Title = fmt.Sprintf("Edit Key: %s", key)
		v.keyModal.Show()
		v.keyValueEditor.RequestFocus()

		if v.App != nil {
			v.App.Invalidate()
		}
	}()
}

func (v *KVView) handleSaveKey() bool {
	if v.App == nil || v.App.NATS() == nil {
		return false
	}

	value := v.keyValueEditor.GetText()

	if v.keyEditMode {
		// Edit mode - check if value changed
		key := v.selectedKeyForEdit
		if key == "" {
			v.App.ShowToast("No key selected for editing", components.ToastTypeError)
			return false
		}

		// If value hasn't changed, do nothing and close modal
		if value == v.keyOriginalValue {
			return true
		}

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Use the NATS client's UpdateKeyValue method
			kv, err := v.App.NATS().GetJetStream().KeyValue(ctx, v.selectedBucket)
			if err != nil {
				v.App.ShowToast(fmt.Sprintf("Failed to get KV bucket: %v", err), components.ToastTypeError)
				return
			}

			// Get current revision and update
			entry, err := kv.Get(ctx, key)
			if err != nil {
				v.App.ShowToast(fmt.Sprintf("Failed to get current revision: %v", err), components.ToastTypeError)
				return
			}
			_, err = kv.Update(ctx, key, []byte(value), entry.Revision())

			if err != nil {
				v.App.ShowToast(fmt.Sprintf("Failed to update key: %v", err), components.ToastTypeError)
				log.Logger().Error().
					Str("bucket", v.selectedBucket).
					Str("key", key).
					Int("value_size", len(value)).
					Uint64("revision", entry.Revision()).
					Err(err).
					Msg("KV key update failed")
				return
			}

			v.App.ShowToast("Key updated successfully", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("bucket", v.selectedBucket).
				Str("key", key).
				Int("value_size", len(value)).
				Uint64("revision", entry.Revision()).
				Msg("KV key updated")
			v.loadBucketKeys(v.selectedBucket)

			if v.App.GetCurrentPageID() == navigator.KVPageId {
				v.App.Invalidate()
			}
		}()
	} else {
		// Create mode - use CAS create
		key := v.keyNameInput.GetText()
		if key == "" {
			v.App.ShowToast("Key name is required", components.ToastTypeError)
			return false
		}

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			kv, err := v.App.NATS().GetJetStream().KeyValue(ctx, v.selectedBucket)
			if err != nil {
				v.App.ShowToast(fmt.Sprintf("Failed to get KV bucket: %v", err), components.ToastTypeError)
				return
			}

			_, err = kv.Create(ctx, key, []byte(value))
			if err != nil {
				v.App.ShowToast(fmt.Sprintf("Failed to create key (may already exist): %v", err), components.ToastTypeError)
				log.Logger().Error().
					Str("bucket", v.selectedBucket).
					Str("key", key).
					Int("value_size", len(value)).
					Err(err).
					Msg("KV key creation failed")
				return
			}

			v.App.ShowToast("Key created successfully", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("bucket", v.selectedBucket).
				Str("key", key).
				Int("value_size", len(value)).
				Msg("KV key created")
			v.loadBucketKeys(v.selectedBucket)

			if v.App.GetCurrentPageID() == navigator.KVPageId {
				v.App.Invalidate()
			}
		}()
	}

	return true
}

func (v *KVView) Refresh() {
	if v.App == nil || v.Loading {
		return
	}

	client := v.App.NATS()
	if client == nil || !client.IsConnected() {
		v.stores = []*KVStoreInfo{}
		v.EmptyState = true
		v.filterStores()
		return
	}

	v.Loading = true
	go func() {
		defer func() {
			v.Loading = false
			if v.App != nil && v.App.GetCurrentPageID() == navigator.KVPageId {
				v.App.Invalidate()
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		stores, err := client.ListKeyValueStores(ctx)
		if err != nil {
			v.App.ShowToast("Failed to list KV stores: "+err.Error(), components.ToastTypeError)
			return
		}

		newStores := make([]*KVStoreInfo, 0, len(stores))
		for _, s := range stores {
			keysCount := int64(s.Values())
			// Get all keys including deleted ones
			entries, err := client.ListKeyValueEntries(ctx, s.Bucket())
			if err == nil {
				keysCount = int64(len(entries))
			}

			newStores = append(newStores, &KVStoreInfo{
				Name:    s.Bucket(),
				Bucket:  s.Bucket(),
				Keys:    keysCount,
				Values:  int64(s.Values()),
				History: s.History() > 1,
				Storage: "File",
				Status:  "Active",
				Created: "",
			})
		}

		v.stores = newStores
		v.EmptyState = len(newStores) == 0
		v.filterStores()
	}()
}

func (v *KVView) filterStores() {
	query := strings.ToLower(v.filterInput.GetText())
	if query == "" {
		v.filtered = v.stores
	} else {
		v.filtered = make([]*KVStoreInfo, 0)
		for _, s := range v.stores {
			if strings.Contains(strings.ToLower(s.Name), query) {
				v.filtered = append(v.filtered, s)
			}
		}
	}
	totalPages := (len(v.filtered) + v.PerPage - 1) / v.PerPage
	v.Paginator.SetTotalPages(totalPages)
	v.Table.ResetWidths()
}

// convertKVItemsToViewerItems converts KV items to MessageViewerItems
func (v *KVView) convertKVItemsToViewerItems(items []models.KVItem) []components.MessageViewerItem {
	viewerItems := make([]components.MessageViewerItem, len(items))
	for i, item := range items {
		subtitle := fmt.Sprintf("Revision: %d\nCreated: %s", item.Revision, item.Created.Format(time.DateTime))
		icon := icons.ActionLabel
		if item.Deleted {
			icon = icons.NavigationClose
		}
		viewerItems[i] = components.MessageViewerItem{
			ID:       item.Key,
			Title:    item.Key,
			Subtitle: subtitle,
			Format:   item.Format,
			Deleted:  item.Deleted,
			Created:  item.Created,
			Icon:     icon,
		}
	}
	return viewerItems
}

// loadKeyValue loads the value for a key and returns it as a string
func (v *KVView) loadKeyValue(key string) string {
	if v.App == nil || v.App.NATS() == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data, err := v.App.NATS().GetKeyValue(ctx, v.selectedBucket, key)
	if err != nil {
		return fmt.Sprintf("Error loading value: %v", err)
	}

	return string(data)
}

func (v *KVView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.RestoreListFocus {
		v.RestoreListFocus = false
		if v.itemsModal.IsOpen {
			gtx.Execute(key.FocusCmd{Tag: v.itemsModal.FocusTag()})
		} else {
			gtx.Execute(key.FocusCmd{Tag: v.Table.FocusTag()})
		}
	}

	for v.AddBtn.Clicked(gtx) {
		v.showCreateBucketModal()
	}

	if v.filterInput.Changed() {
		v.LastFilterTime = gtx.Now
	}

	if !v.LastFilterTime.IsZero() {
		if gtx.Now.Sub(v.LastFilterTime) > 300*time.Millisecond {
			v.filterStores()
			v.LastFilterTime = time.Time{}
		} else {
			gtx.Execute(op.InvalidateCmd{At: v.LastFilterTime.Add(300 * time.Millisecond)})
		}
	}

	for v.RefreshBtn.Clicked(gtx) {
		v.Refresh()
	}

	for v.browseBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.loadBucketKeys(v.filtered[v.SelectedIdx].Bucket)
		}
	}

	for v.DeleteBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.promptBucketDelete(v.filtered[v.SelectedIdx].Bucket)
		}
	}

	for v.purgeBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.promptBucketPurge(v.filtered[v.SelectedIdx].Bucket)
		}
	}

	// Only handle TAB navigation when no modal is open
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
	}

	// Handle table keyboard shortcuts (only when no modal is open)
	if !v.isModalVisible() {
		for {
			ev, ok := gtx.Event(
				key.Filter{Focus: v.Table.FocusTag(), Name: key.NameDeleteForward, Optional: key.ModShift},
				key.Filter{Focus: v.Table.FocusTag(), Name: key.NameDeleteBackward, Optional: key.ModShift},
			)
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press {
				if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
					if e.Modifiers.Contain(key.ModShift) {
						v.promptBucketPurge(v.filtered[v.SelectedIdx].Bucket)
					} else {
						v.promptBucketDelete(v.filtered[v.SelectedIdx].Bucket)
					}
				}
			}
		}
	}

	for v.watchBtn.Clicked(gtx) {
		if v.App != nil {
			v.App.ShowToast("Watch mode coming soon", components.ToastTypeInfo)
			log.Logger().Info().
				Str("bucket", v.selectedBucket).
				Str("action", "watch_bucket").
				Msg("KV bucket watch requested")
		}
	}

	for v.addKeyBtn.Clicked(gtx) {
		if v.itemsModal.IsOpen && v.selectedBucket != "" {
			v.showAddKeyModal()
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
				v.loadBucketKeys(v.filtered[newIdx].Bucket)
				v.App.Invalidate()
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

	layout.UniformInset(unit.Dp(32)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		if v.itemsModal.IsOpen || v.createModal.Visible {
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

	// Check for pending modal open (from background goroutine)
	if v.pendingModalItems != nil {
		items := v.pendingModalItems
		title := v.pendingModalTitle
		v.pendingModalItems = nil
		v.pendingModalTitle = ""
		v.itemsModal.Open(title, items)
	}

	// Layout the items modal using the reusable component
	// Layout all modals using Stack to ensure proper layering
	// The last modal in the stack will be on top and capture events
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.itemsModal.IsOpen {
				return v.itemsModal.Layout(cgtx)
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
			if v.keyModal.Visible {
				return v.keyModal.Layout(cgtx, th)
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

func (v *KVView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(24), "Key-Value Stores")
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

func (v *KVView) layoutActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	isSelected := v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			btn := components.Button(th, &v.AddBtn, icons.ContentAddCircle, components.IconPositionStart, "Create Bucket")
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
			btn := components.SecondaryButton(th, &v.browseBtn, icons.ActionVisibility, components.IconPositionStart, "Browse")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.watchBtn, icons.ActionSearch, components.IconPositionStart, "Watch")
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
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.filterInput.Layout(cgtx, th)
		}),
	)
}

func (v *KVView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.EmptyState {
		return v.layoutEmptyState(gtx, th)
	}

	return v.Split.Layout(gtx, th,
		func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
					return v.layoutStoresTable(ccgtx, th)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					return v.Paginator.Layout(ccgtx, th)
				}),
			)
		},
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutKVDetails(cgtx, th)
		},
	)
}

func (v *KVView) layoutKVDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.filtered) {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Select a store")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	store := v.filtered[v.SelectedIdx]
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.Card{
					Title: store.Name,
				}.Layout(ccgtx, th, func(cccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							stateType := components.StatusPillNeutral
							switch store.Status {
							case "Active":
								stateType = components.StatusPillSuccess
							case "Idle":
								stateType = components.StatusPillWarning
							}
							return components.StatusPill{
								Text: store.Status,
								Type: stateType,
							}.Layout(c4gtx, th)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layout.Flex{Spacing: layout.SpaceBetween}.Layout(c4gtx,
								layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
									return components.StatCard{
										Title: "Keys",
										Value: fmt.Sprintf("%d", store.Keys),
									}.Layout(c5gtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
									return components.StatCard{
										Title: "Values",
										Value: fmt.Sprintf("%d", store.Values),
									}.Layout(c5gtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
									return components.StatCard{
										Title: "Replicas",
										Value: fmt.Sprintf("%d", store.Replicas),
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
							return layoutDetailRow(c4gtx, th, "Bucket", store.Bucket)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							history := "No"
							if store.History {
								history = "Yes"
							}
							return layoutDetailRow(c4gtx, th, "History", history)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Max Age", store.MaxAge)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Storage", store.Storage)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Created", store.Created)
						}),
					)
				})
			}),
		)
	})
}

func (v *KVView) layoutStoresTable(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	startIdx := (v.Paginator.CurrentPage - 1) * v.PerPage
	endIdx := startIdx + v.PerPage
	if endIdx > len(v.filtered) {
		endIdx = len(v.filtered)
	}
	if startIdx < 0 || startIdx >= len(v.filtered) {
		startIdx = 0
		endIdx = 0
	}

	var pageStores []*KVStoreInfo
	if endIdx > startIdx {
		pageStores = v.filtered[startIdx:endIdx]
	}

	v.Table.Rows = make([]components.TableRow, len(pageStores))
	for i, s := range pageStores {
		historyText := "No"
		if s.History {
			historyText = "Yes"
		}
		v.Table.Rows[i] = components.TableRow{
			Values: []string{
				s.Name,
				fmt.Sprintf("%d", s.Keys),
				historyText,
				s.Storage,
				s.Status,
			},
			Selected: (startIdx + i) == v.SelectedIdx,
		}
	}

	return v.Table.Layout(gtx, th)
}

func (v *KVView) layoutEmptyState(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return components.EmptyState{
		Icon:    icons.ActionLabel,
		Title:   "No KV Stores Found",
		Message: "Create a bucket to store key-value pairs.",
	}.Layout(gtx, th)
}

func (v *KVView) promptBucketDelete(bucket string) {
	v.ConfirmModal.Title = "Delete Bucket"
	v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to delete bucket '%s'?", bucket)
	v.ConfirmModal.ReturnFocus = v.Table.FocusTag()
	v.ConfirmModal.SetOnClose(func() {
		v.RestoreListFocus = true
	})
	v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
		if option == "Confirm" {
			v.deleteBucket(bucket)
		}
	})
	v.ConfirmModal.Show()
}

func (v *KVView) promptBucketPurge(bucket string) {
	v.ConfirmModal.Title = "Purge Bucket"
	v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to purge all values from bucket '%s'?", bucket)
	v.ConfirmModal.ReturnFocus = v.Table.FocusTag()
	v.ConfirmModal.SetOnClose(func() {
		v.RestoreListFocus = true
	})
	v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
		if option == "Confirm" {
			v.purgeBucket(bucket)
		}
	})
	v.ConfirmModal.Show()
}

func (v *KVView) promptKeyDelete(key string) {
	v.ConfirmModal.Title = "Delete Key"
	v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to delete key '%s'?", key)
	v.ConfirmModal.ReturnFocus = v.itemsModal.FocusTag()
	v.ConfirmModal.SetOnClose(func() {
		v.RestoreListFocus = true
	})
	v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
		if option == "Confirm" {
			v.deleteKey(v.selectedBucket, key)
		}
	})
	v.ConfirmModal.Show()
}

func (v *KVView) promptKeyPurge(key string) {
	v.ConfirmModal.Title = "Purge Key"
	v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to purge all versions of key '%s'?", key)
	v.ConfirmModal.ReturnFocus = v.itemsModal.FocusTag()
	v.ConfirmModal.SetOnClose(func() {
		v.RestoreListFocus = true
	})
	v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
		if option == "Confirm" {
			v.purgeKey(v.selectedBucket, key)
		}
	})
	v.ConfirmModal.Show()
}

func (v *KVView) loadBucketKeys(bucket string) {
	if v.App == nil || v.App.NATS() == nil {
		return
	}

	v.selectedBucket = bucket
	v.bucketItems = []models.KVItem{}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		entries, err := v.App.NATS().ListKeyValueEntries(ctx, bucket)
		if err != nil {
			v.App.ShowToast("Failed to list keys: "+err.Error(), components.ToastTypeError)
			return
		}

		items := make([]models.KVItem, 0, len(entries))
		for _, e := range entries {
			isDeleted := e.Operation() != jetstream.KeyValuePut
			format := ""
			if !isDeleted {
				format = utils.DetectPayloadFormat(e.Value())
			}
			items = append(items, models.KVItem{
				Key:      e.Key(),
				Format:   format,
				Deleted:  isDeleted,
				Revision: e.Revision(),
				Created:  e.Created(),
			})
		}

		sort.Slice(items, func(i, j int) bool {
			return items[i].Revision > items[j].Revision
		})

		v.bucketItems = items

		// Convert to viewer items and open modal
		viewerItems := v.convertKVItemsToViewerItems(items)

		// Store the items and bucket name for opening on the main thread
		v.pendingModalItems = viewerItems
		v.pendingModalTitle = fmt.Sprintf("Items in %s", bucket)

		if v.App != nil && v.App.GetCurrentPageID() == navigator.KVPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *KVView) IsItemsModalOpen() bool {
	return v.itemsModal.IsOpen
}

func (v *KVView) CloseItemsModal() {
	v.itemsModal.Close()
	if v.App != nil {
		v.App.Invalidate()
	}
}

func (v *KVView) isModalVisible() bool {
	return v.itemsModal.IsOpen || v.createModal.Visible || v.keyModal.Visible || v.ConfirmModal.IsVisible()
}

func (v *KVView) handleTab(gtx layout.Context, shift bool) {
	var tags []any
	if v.isModalVisible() {
		// Tab navigation is handled by the modal itself
		return
	} else {
		tags = []any{
			&v.AddBtn,
			&v.RefreshBtn,
		}

		isSelected := v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered)
		if isSelected {
			tags = append(tags, &v.browseBtn, &v.watchBtn, &v.DeleteBtn, &v.purgeBtn)
		}

		tags = append(tags, v.filterInput.FocusTag())

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

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *KVView) HandleShortcuts(gtx layout.Context) bool {
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
func (v *KVView) GetShortcutsHelp() []shortcuts.Shortcut {
	return []shortcuts.Shortcut{
		shortcuts.Create(func() {}),
		shortcuts.Refresh(func() {}),
		shortcuts.Delete(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.Browse(func() bool { return v.SelectedIdx >= 0 }, func() {}),
	}
}
