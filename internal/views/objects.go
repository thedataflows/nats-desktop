package views

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sqweek/dialog"
	log "github.com/thedataflows/go-lib-log"
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

	"github.com/nats-io/nats.go/jetstream"
	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/navigator"

	"github.com/thedataflows/nats-desktop/internal/shortcuts"
)

type ObjectsView struct {
	*BaseView

	stores   []*ObjectStore
	filtered []*ObjectStore

	// Extra buttons not in BaseView
	uploadBtn widget.Clickable
	browseBtn widget.Clickable

	// Create Bucket Modal
	createModal *components.FormModal
	bucketInput *components.InputField

	// Filter chips
	allFilter    *components.FilterChip
	imagesFilter *components.FilterChip
	docsFilter   *components.FilterChip

	// Object Browser Modal
	browserModal       *components.FormModal
	selectedBucket     string
	objects            []*jetstream.ObjectInfo
	selectedObjectIdx  int
	browserRefreshBtn  widget.Clickable
	browserUploadBtn   widget.Clickable
	browserDownloadBtn widget.Clickable
	browserDeleteBtn   widget.Clickable
	browserInfoBtn     widget.Clickable
	objectsLoading     bool
	objectsError       string
	objectsList        *components.ListStyle
	lastObjectsCount   int
	lastObjectsHash    string

	// Upload Modal
	uploadModal      *components.FormModal
	uploadObjectName *components.InputField
	uploadData       []byte
	uploadFileName   string

	// Object Info Modal
	infoModal   *components.FormModal
	infoResults *jetstream.ObjectInfo
	infoLoading bool

	next, prev any
}

type ObjectStore struct {
	Name    string
	Files   int
	Size    string
	Storage string
	Status  string
	Created string
}

func NewObjectsView(th *theme.Theme) *ObjectsView {
	v := &ObjectsView{
		BaseView: NewBaseView(
			[]string{"Name", "Files", "Size", "Storage", "Status", "Created"},
			15,
		),
		allFilter:         components.NewFilterChip("All"),
		imagesFilter:      components.NewFilterChip("Images"),
		docsFilter:        components.NewFilterChip("Documents"),
		bucketInput:       components.NewLabeledInputFieldWithPosition("Store name", "", components.LabelPositionTop),
		uploadObjectName:  components.NewLabeledInputFieldWithPosition("Object name", "", components.LabelPositionTop),
		selectedObjectIdx: -1,
	}
	v.allFilter.SetSelected(true)

	v.SearchEditor.Placeholder = "Search object stores..."
	v.SearchEditor.SetIcon(icons.ActionSearch, components.IconPositionStart)

	// Initialize modals
	v.createModal = components.NewFormModal("Create Object Store")
	v.createModal.MaxWidth = unit.Dp(400)
	v.createModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.bucketInput.Layout(gtx, th)
	}
	v.createModal.CustomFocusTags = []event.Tag{
		v.bucketInput.FocusTag(),
	}
	v.createModal.OnSave = func() bool {
		bucket := v.bucketInput.GetText()
		if bucket != "" {
			v.createBucket(bucket)
			v.RestoreListFocus = true
			return true
		}
		return false
	}
	v.createModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.createModal.ReturnFocus = v.Table.FocusTag()

	v.browserModal = components.NewFormModal("Object Browser")
	v.browserModal.MaxWidth = unit.Dp(500)
	v.browserModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.layoutBrowserModalContent(gtx, th)
	}
	v.browserModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.browserModal.ReturnFocus = v.Table.FocusTag()
	v.browserModal.MaxHeight = unit.Dp(500)
	v.browserModal.HideSaveButton = true
	v.browserModal.OnEnter = func() {
		if v.selectedObjectIdx >= 0 && v.selectedObjectIdx < len(v.objects) {
			v.downloadObject(v.objects[v.selectedObjectIdx].Name)
		}
	}

	v.infoModal = components.NewFormModal("Object Information")
	v.infoModal.MaxWidth = unit.Dp(450)
	v.infoModal.CustomContent = func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
		return v.layoutInfoModalContent(gtx, th)
	}
	v.infoModal.OnClose = func() {
		v.RestoreListFocus = true
	}
	v.infoModal.ReturnFocus = v.Table.FocusTag()
	v.infoModal.HideSaveButton = true

	// Initialize objects list for browser modal
	v.objectsList = components.NewList(th)
	v.objectsList.OnSelect(func(index int) {
		v.selectedObjectIdx = index
		if v.App != nil {
			v.App.Invalidate()
		}
	})

	// Track last click time for double-click detection
	var lastClickTime time.Time
	var lastClickIndex int
	v.objectsList.OnClick(func(index int) {
		v.selectedObjectIdx = index

		// Check for double-click (within 300ms)
		now := time.Now()
		if index == lastClickIndex && now.Sub(lastClickTime) < 300*time.Millisecond {
			// Double-click detected - download the object
			if v.App != nil && index >= 0 && index < len(v.objects) {
				v.downloadObject(v.objects[index].Name)
			}
		}
		lastClickTime = now
		lastClickIndex = index

		if v.App != nil {
			v.App.Invalidate()
		}
	})

	// Add Enter key handler via CustomFocusTags - the browser modal will handle it
	// Note: The ListStyle already handles Enter key via OnClick, which includes double-click detection

	// Add list focus tag to browser modal for keyboard navigation
	v.browserModal.CustomFocusTags = []event.Tag{v.objectsList.FocusTag()}

	return v
}

func (v *ObjectsView) SetApp(app App) {
	v.App = app
	v.Table.OnCopyFeedback = func(text string) {
		if v.App != nil {
			v.App.ShowToast("Copied: "+text, components.ToastTypeSuccess)
		}
	}
	v.objectsList.OnCopyFeedback = func(text string) {
		if v.App != nil {
			v.App.ShowToast("Copied: "+text, components.ToastTypeSuccess)
		}
	}
}

func (v *ObjectsView) SetNavigation(next, prev any) {
	v.next = next
	v.prev = prev
}

func (v *ObjectsView) Info() navigator.Info {
	return navigator.Info{
		ID:    navigator.ObjectsPageId,
		Title: "Objects",
		Icon:  icons.FileCloudUpload,
	}
}

func (v *ObjectsView) OnEnter() {
	v.Refresh()
}

func (v *ObjectsView) FirstFocusTag() any {
	return v.SearchEditor.FocusTag()
}

func (v *ObjectsView) LastFocusTag() any {
	return v.Paginator.NextClick
}

func (v *ObjectsView) createBucket(bucket string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := v.App.NATS().CreateObjectStore(ctx, jetstream.ObjectStoreConfig{
			Bucket: bucket,
		})
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to create store: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("bucket", bucket).
				Err(err).
				Msg("Object store creation failed")
		} else {
			v.App.ShowToast("Object store created successfully", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("bucket", bucket).
				Msg("Object store created")
			v.Refresh()
		}
		if v.App != nil && v.App.GetCurrentPageID() == navigator.ObjectsPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ObjectsView) deleteBucket(bucket string) {
	if v.App == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := v.App.NATS().DeleteObjectStore(ctx, bucket)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to delete store: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("bucket", bucket).
				Err(err).
				Msg("Object store deletion failed")
		} else {
			v.App.ShowToast("Object store deleted", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("bucket", bucket).
				Msg("Object store deleted")
			v.SelectedIdx = -1
			v.Refresh()
		}
		if v.App != nil && v.App.GetCurrentPageID() == navigator.ObjectsPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ObjectsView) isModalVisible() bool {
	return v.createModal.Visible || v.browserModal.Visible || v.infoModal.Visible || v.ConfirmModal.IsVisible()
}

// isTopModal returns true if the given modal is the topmost visible modal
func (v *ObjectsView) isTopModal(modal string) bool {
	switch modal {
	case "info":
		// Info modal is always on top if visible
		return v.infoModal.Visible
	case "browser":
		// Browser modal is on top only if visible and info is not visible
		return v.browserModal.Visible && !v.infoModal.Visible
	case "create":
		// Create modal is on top only if visible and browser/info are not visible
		return v.createModal.Visible && !v.browserModal.Visible && !v.infoModal.Visible
	default:
		return false
	}
}

func (v *ObjectsView) openBrowser(bucket string) {
	v.selectedBucket = bucket
	v.browserModal.Title = fmt.Sprintf("Objects in %s", bucket)
	v.browserModal.Show()
	v.selectedObjectIdx = -1
	v.objects = nil
	v.objectsError = ""
	v.refreshObjects()
}

func (v *ObjectsView) refreshObjects() {
	if v.App == nil || v.selectedBucket == "" {
		return
	}

	v.objectsLoading = true
	v.objectsError = ""
	if v.App != nil {
		v.App.Invalidate()
	}

	go func() {
		defer func() {
			v.objectsLoading = false
			if v.App != nil && v.App.GetCurrentPageID() == navigator.ObjectsPageId {
				v.App.Invalidate()
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		objects, err := v.App.NATS().ListObjects(ctx, v.selectedBucket)
		if err != nil {
			v.objectsError = err.Error()
			return
		}

		v.objects = objects
	}()
}

func (v *ObjectsView) deleteObject(objectName string) {
	if v.App == nil || v.selectedBucket == "" {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Get object info before deletion for logging
		objInfo, _ := v.App.NATS().GetObjectInfo(ctx, v.selectedBucket, objectName)

		err := v.App.NATS().DeleteObject(ctx, v.selectedBucket, objectName)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to delete object: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("bucket", v.selectedBucket).
				Str("object", objectName).
				Err(err).
				Msg("Object deletion failed")
		} else {
			v.App.ShowToast("Object deleted successfully", components.ToastTypeSuccess)
			logger := log.Logger().Info().
				Str("bucket", v.selectedBucket).
				Str("object", objectName)
			if objInfo != nil {
				logger = logger.Uint64("size", objInfo.Size)
			}
			logger.Msg("Object deleted")
			v.refreshObjects()
		}

		if v.App != nil && v.App.GetCurrentPageID() == navigator.ObjectsPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ObjectsView) showObjectInfo(objectName string) {
	if v.App == nil || v.selectedBucket == "" {
		return
	}

	v.infoModal.Title = fmt.Sprintf("Object Information: %s", objectName)
	v.infoModal.Show()
	v.infoLoading = true
	v.infoResults = nil

	go func() {
		defer func() { v.infoLoading = false }()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		info, err := v.App.NATS().GetObjectInfo(ctx, v.selectedBucket, objectName)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to get object info: %v", err), components.ToastTypeError)
			return
		}

		v.infoResults = info

		log.Logger().Info().
			Str("bucket", v.selectedBucket).
			Str("object", objectName).
			Uint64("size", info.Size).
			Time("modified", info.ModTime).
			Msg("Object info viewed")

		if v.App != nil && v.App.GetCurrentPageID() == navigator.ObjectsPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ObjectsView) uploadObject(objectName string, data []byte) {
	if v.App == nil || v.selectedBucket == "" {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, err := v.App.NATS().PutObject(ctx, v.selectedBucket, objectName, data)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to upload object: %v", err), components.ToastTypeError)
			log.Logger().Error().
				Str("bucket", v.selectedBucket).
				Str("object", objectName).
				Int("size", len(data)).
				Err(err).
				Msg("Object upload failed")
		} else {
			v.App.ShowToast("Object uploaded successfully", components.ToastTypeSuccess)
			log.Logger().Info().
				Str("bucket", v.selectedBucket).
				Str("object", objectName).
				Int("size", len(data)).
				Msg("Object uploaded")
			v.refreshObjects()
		}

		if v.App != nil && v.App.GetCurrentPageID() == navigator.ObjectsPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ObjectsView) downloadObject(objectName string) {
	if v.App == nil || v.selectedBucket == "" {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		data, err := v.App.NATS().GetObject(ctx, v.selectedBucket, objectName)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to download object: %v", err), components.ToastTypeError)
			return
		}

		// Open save file dialog
		savePath, err := dialog.File().Title("Save Object").SetStartFile(objectName).Save()
		if err != nil {
			// User cancelled or error occurred
			return
		}

		// Save the file
		err = os.WriteFile(savePath, data, 0644)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to save file: %v", err), components.ToastTypeError)
			return
		}

		v.App.ShowToast(fmt.Sprintf("Saved %s (%d bytes)", objectName, len(data)), components.ToastTypeSuccess)
		log.Logger().Info().
			Str("bucket", v.selectedBucket).
			Str("object", objectName).
			Int("size", len(data)).
			Str("save_path", savePath).
			Msg("Object downloaded")

		if v.App != nil && v.App.GetCurrentPageID() == navigator.ObjectsPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ObjectsView) handleUploadFiles() {
	if v.App == nil || v.selectedBucket == "" {
		return
	}

	// Open file picker dialog for single file selection
	go func() {
		filePath, err := dialog.File().Title("Select a file to upload").Load()
		if err != nil {
			// User cancelled or error occurred
			return
		}

		// Read the file
		data, err := os.ReadFile(filePath)
		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to read file %s: %v", filepath.Base(filePath), err), components.ToastTypeError)
			return
		}

		// Use the filename as the object name
		objectName := filepath.Base(filePath)

		// Upload to NATS
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err = v.App.NATS().PutObject(ctx, v.selectedBucket, objectName, data)
		cancel()

		if err != nil {
			v.App.ShowToast(fmt.Sprintf("Failed to upload %s: %v", objectName, err), components.ToastTypeError)
			log.Logger().Error().
				Str("bucket", v.selectedBucket).
				Str("object", objectName).
				Int("size", len(data)).
				Str("source_file", filePath).
				Err(err).
				Msg("Object upload failed")
		} else {
			v.App.ShowToast(fmt.Sprintf("Successfully uploaded %s", objectName), components.ToastTypeSuccess)
			log.Logger().Info().
				Str("bucket", v.selectedBucket).
				Str("object", objectName).
				Int("size", len(data)).
				Str("source_file", filePath).
				Msg("Object uploaded")
			v.refreshObjects()
			v.Refresh() // Refresh the stores list to update counts
		}

		if v.App != nil && v.App.GetCurrentPageID() == navigator.ObjectsPageId {
			v.App.Invalidate()
		}
	}()
}

func (v *ObjectsView) formatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func (v *ObjectsView) Refresh() {
	if v.App == nil || v.Loading {
		return
	}

	client := v.App.NATS()
	if client == nil || !client.IsConnected() {
		v.stores = []*ObjectStore{}
		v.EmptyState = true
		return
	}

	v.Loading = true
	go func() {
		defer func() {
			v.Loading = false
			if v.App != nil && v.App.GetCurrentPageID() == navigator.ObjectsPageId {
				v.App.Invalidate()
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		stores, err := client.ListObjectStores(ctx)
		if err != nil {
			v.App.ShowToast("Failed to list object stores: "+err.Error(), components.ToastTypeError)
			return
		}

		newStores := make([]*ObjectStore, 0, len(stores))
		for _, s := range stores {
			// Get object count by listing objects
			objCount := 0
			objects, err := client.ListObjects(ctx, s.Bucket())
			if err == nil {
				objCount = len(objects)
			}

			// Get created date from stream info
			created := ""
			streamInfo, err := client.GetObjectStoreStreamInfo(ctx, s.Bucket())
			if err == nil && streamInfo != nil {
				created = streamInfo.Created.Format(time.DateTime)
			}

			newStores = append(newStores, &ObjectStore{
				Name:    s.Bucket(),
				Files:   objCount,
				Size:    v.formatBytes(s.Size()),
				Storage: "File",
				Status:  "Active",
				Created: created,
			})
		}

		v.stores = newStores
		v.EmptyState = len(newStores) == 0
		v.filterStores()
	}()
}

func (v *ObjectsView) filterStores() {
	query := strings.ToLower(v.SearchEditor.GetText())
	v.filtered = make([]*ObjectStore, 0)

	for _, store := range v.stores {
		// Check search query
		if query != "" && !strings.Contains(strings.ToLower(store.Name), query) {
			continue
		}

		// Check filters - include if no filters selected OR if store matches a selected filter
		if !v.allFilter.Selected && !v.imagesFilter.Selected && !v.docsFilter.Selected {
			// No filters selected, show all
			v.filtered = append(v.filtered, store)
		} else if v.allFilter.Selected {
			v.filtered = append(v.filtered, store)
		} else if v.imagesFilter.Selected {
			// Filter for image files
			if strings.HasSuffix(strings.ToLower(store.Name), ".jpg") ||
				strings.HasSuffix(strings.ToLower(store.Name), ".jpeg") ||
				strings.HasSuffix(strings.ToLower(store.Name), ".png") ||
				strings.HasSuffix(strings.ToLower(store.Name), ".gif") {
				v.filtered = append(v.filtered, store)
			}
		} else if v.docsFilter.Selected {
			// Filter for document files
			if strings.HasSuffix(strings.ToLower(store.Name), ".pdf") ||
				strings.HasSuffix(strings.ToLower(store.Name), ".doc") ||
				strings.HasSuffix(strings.ToLower(store.Name), ".docx") ||
				strings.HasSuffix(strings.ToLower(store.Name), ".txt") {
				v.filtered = append(v.filtered, store)
			}
		}
		// If none of the above, store is filtered out
	}

	totalPages := (len(v.filtered) + v.PerPage - 1) / v.PerPage
	v.Paginator.SetTotalPages(totalPages)
	v.Table.ResetWidths()

	// Trigger UI refresh after filtering
	if v.App != nil {
		v.App.Invalidate()
	}
}

func (v *ObjectsView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.RestoreListFocus {
		v.RestoreListFocus = false
		gtx.Execute(key.FocusCmd{Tag: v.Table.FocusTag()})
	}

	for v.AddBtn.Clicked(gtx) {
		v.createModal.Show()
		v.bucketInput.SetText("")
	}

	// Handle TAB navigation only when no modal is open
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

	for v.RefreshBtn.Clicked(gtx) {
		v.Refresh()
	}

	for v.DeleteBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.stores) {
			bucket := v.stores[v.SelectedIdx].Name
			v.ConfirmModal.Title = "Delete Object Store"
			v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to delete object store '%s'?", bucket)
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
	}

	for v.browseBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.openBrowser(v.filtered[v.SelectedIdx].Name)
		}
	}

	for v.uploadBtn.Clicked(gtx) {
		if v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
			v.selectedBucket = v.filtered[v.SelectedIdx].Name
			go v.handleUploadFiles()
		}
	}

	// Handle browser modal buttons
	if v.browserModal.Visible {
		for v.browserRefreshBtn.Clicked(gtx) {
			v.refreshObjects()
		}

		for v.browserUploadBtn.Clicked(gtx) {
			go v.handleUploadFiles()
		}

		for v.browserDownloadBtn.Clicked(gtx) {
			if v.selectedObjectIdx >= 0 && v.selectedObjectIdx < len(v.objects) {
				v.downloadObject(v.objects[v.selectedObjectIdx].Name)
			}
		}

		for v.browserDeleteBtn.Clicked(gtx) {
			if v.selectedObjectIdx >= 0 && v.selectedObjectIdx < len(v.objects) {
				objName := v.objects[v.selectedObjectIdx].Name
				v.ConfirmModal.Title = "Delete Object"
				v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to delete object '%s'?", objName)
				v.ConfirmModal.ReturnFocus = v.Table.FocusTag()
				v.ConfirmModal.SetOnClose(func() {})
				v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
					if option == "Confirm" {
						v.deleteObject(objName)
					}
				})
				v.ConfirmModal.Show()
			}
		}

		for v.browserInfoBtn.Clicked(gtx) {
			if v.selectedObjectIdx >= 0 && v.selectedObjectIdx < len(v.objects) {
				v.showObjectInfo(v.objects[v.selectedObjectIdx].Name)
			}
		}

		// Handle Delete key in browser modal (only when confirmation modal is not open)
		if !v.ConfirmModal.IsVisible() {
			for {
				ev, ok := gtx.Event(key.Filter{Name: key.NameDeleteForward}, key.Filter{Name: key.NameDeleteBackward})
				if !ok {
					break
				}
				if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
					if v.selectedObjectIdx >= 0 && v.selectedObjectIdx < len(v.objects) {
						objName := v.objects[v.selectedObjectIdx].Name
						v.ConfirmModal.Title = "Delete Object"
						v.ConfirmModal.Content = fmt.Sprintf("Are you sure you want to delete object '%s'?", objName)
						v.ConfirmModal.ReturnFocus = v.Table.FocusTag()
						v.ConfirmModal.SetOnClose(func() {})
						v.ConfirmModal.SetOnSubmit(func(option string, _ bool) {
							if option == "Confirm" {
								v.deleteObject(objName)
							}
						})
						v.ConfirmModal.Show()
					}
				}
			}
		}
	}

	if v.SearchEditor.Changed() {
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

	for v.allFilter.Click.Clicked(gtx) {
		if v.allFilter.Selected {
			v.allFilter.SetSelected(false)
		} else {
			v.allFilter.SetSelected(true)
			v.imagesFilter.SetSelected(false)
			v.docsFilter.SetSelected(false)
		}
		v.filterStores()
	}

	for v.imagesFilter.Click.Clicked(gtx) {
		if v.imagesFilter.Selected {
			v.imagesFilter.SetSelected(false)
		} else {
			v.imagesFilter.SetSelected(true)
			v.allFilter.SetSelected(false)
			v.docsFilter.SetSelected(false)
		}
		v.filterStores()
	}

	for v.docsFilter.Click.Clicked(gtx) {
		if v.docsFilter.Selected {
			v.docsFilter.SetSelected(false)
		} else {
			v.docsFilter.SetSelected(true)
			v.allFilter.SetSelected(false)
			v.imagesFilter.SetSelected(false)
		}
		v.filterStores()
	}

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
				v.openBrowser(v.filtered[newIdx].Name)
			}
		}
		v.SelectedIdx = newIdx
	}
	if v.Table.SelectionChanged() {
		v.SelectedIdx = (v.Paginator.CurrentPage-1)*v.PerPage + v.Table.SelectedRow
	}

	// Handle Enter key to open browser for selected item
	if !v.isModalVisible() && v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered) {
		for {
			ev, ok := gtx.Event(key.Filter{Name: key.NameReturn}, key.Filter{Name: key.NameEnter})
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				v.openBrowser(v.filtered[v.SelectedIdx].Name)
			}
		}
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
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				return v.layoutContent(ccgtx, th)
			}),
		)
	})

	// Render all modals using Stack
	// Order matters: last in stack renders on top and captures events first

	// Block browser modal events when child modals are visible
	v.browserModal.BlockEvents = v.infoModal.Visible || v.ConfirmModal.IsVisible()

	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.createModal.Visible {
				return v.createModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.browserModal.Visible {
				return v.browserModal.Layout(cgtx, th)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			if v.infoModal.Visible {
				return v.infoModal.Layout(cgtx, th)
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

func (v *ObjectsView) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			header := material.Label(th.Material(), unit.Sp(24), "Object Stores")
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

func (v *ObjectsView) layoutActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	isSelected := v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			btn := components.Button(th, &v.AddBtn, icons.ContentAddCircle, components.IconPositionStart, "Create Store")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.browseBtn, icons.ActionSearch, components.IconPositionStart, "Browse")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &v.uploadBtn, icons.FileFileUpload, components.IconPositionStart, "Upload")
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
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return v.SearchEditor.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.allFilter.Layout(cgtx, th)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.imagesFilter.Layout(cgtx, th)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return v.docsFilter.Layout(cgtx, th)
		}),
	)
}

func (v *ObjectsView) layoutContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.EmptyState {
		return v.layoutEmptyState(gtx, th)
	}

	return v.Split.Layout(gtx, th,
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutStoresTable(cgtx, th)
		},
		func(cgtx layout.Context) layout.Dimensions {
			return v.layoutObjectStoreDetails(cgtx, th)
		},
	)
}

func (v *ObjectsView) layoutStoresTable(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	v.Table.Rows = BuildTableRows(v.filtered, v.Paginator.CurrentPage, v.PerPage,
		func(s *ObjectStore, idx int) components.TableRow {
			return components.TableRow{
				Values: []string{
					s.Name,
					fmt.Sprintf("%d", s.Files),
					s.Size,
					s.Storage,
					s.Status,
					s.Created,
				},
			}
		}, v.SelectedIdx)

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

func (v *ObjectsView) layoutObjectStoreDetails(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.SelectedIdx < 0 || v.SelectedIdx >= len(v.stores) {
		return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Select an object store")
			lbl.Color = th.TextColor
			return lbl.Layout(cgtx)
		})
	}

	store := v.stores[v.SelectedIdx]
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return components.Card{
					Title: store.Name,
				}.Layout(ccgtx, th, func(cccgtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Middle}.Layout(c4gtx,
								layout.Rigid(func(c5gtx layout.Context) layout.Dimensions {
									stateType := components.StatusPillNeutral
									switch store.Status {
									case "Active":
										stateType = components.StatusPillSuccess
									case "Offline":
										stateType = components.StatusPillError
									}
									return components.StatusPill{
										Text: store.Status,
										Type: stateType,
									}.Layout(c5gtx, th)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layout.Flex{Spacing: layout.SpaceBetween}.Layout(c4gtx,
								layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
									return components.StatCard{
										Title: "Files",
										Value: fmt.Sprintf("%d", store.Files),
									}.Layout(c5gtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
									return components.StatCard{
										Title: "Size",
										Value: store.Size,
									}.Layout(c5gtx, th)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Flexed(1, func(c5gtx layout.Context) layout.Dimensions {
									return components.StatCard{
										Title: "Storage",
										Value: store.Storage,
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
							return layoutDetailRow(c4gtx, th, "Name", store.Name)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Storage Type", store.Storage)
						}),
						layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
							return layoutDetailRow(c4gtx, th, "Status", store.Status)
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

func (v *ObjectsView) handleTab(gtx layout.Context, shift bool) {
	var tags []any
	if v.createModal.Visible {
		tags = []any{
			v.bucketInput.FocusTag(),
		}
	} else if v.browserModal.Visible {
		tags = []any{
			&v.browserRefreshBtn,
			&v.browserUploadBtn,
		}
		if v.selectedObjectIdx >= 0 && v.selectedObjectIdx < len(v.objects) {
			tags = append(tags,
				&v.browserDownloadBtn,
				&v.browserDeleteBtn,
				&v.browserInfoBtn,
			)
		}
		if v.objectsList != nil {
			tags = append(tags, v.objectsList.FocusTag())
		}
		tags = append(tags, v.browserModal.CancelBtn)
	} else if v.infoModal.Visible {
		tags = []any{}
	} else {
		tags = []any{
			&v.AddBtn,
		}

		isSelected := v.SelectedIdx >= 0 && v.SelectedIdx < len(v.filtered)
		if isSelected {
			tags = append(tags, &v.browseBtn, &v.uploadBtn)
		}

		tags = append(tags, &v.RefreshBtn)

		if isSelected {
			tags = append(tags, &v.DeleteBtn)
		}

		tags = append(tags,
			v.SearchEditor.FocusTag(),
			v.allFilter.FocusTag(),
			v.imagesFilter.FocusTag(),
			v.docsFilter.FocusTag(),
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

func (v *ObjectsView) layoutEmptyState(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return components.EmptyState{
		Icon:    icons.FileCloudUpload,
		Title:   "No Object Stores Found",
		Message: "Create a bucket to store files and objects.",
	}.Layout(gtx, th)
}

func (v *ObjectsView) layoutBrowserModalContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	// Update the objects list with current objects
	v.updateObjectsList(th)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
			isObjectSelected := v.selectedObjectIdx >= 0 && v.selectedObjectIdx < len(v.objects)
			return layout.Flex{Axis: layout.Horizontal}.Layout(c4gtx,
				layout.Rigid(func(c5gtx layout.Context) layout.Dimensions {
					btn := components.SecondaryButton(th, &v.browserRefreshBtn, icons.NavigationRefresh, components.IconPositionStart, "Refresh")
					return btn.Layout(c5gtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(c5gtx layout.Context) layout.Dimensions {
					btn := components.SecondaryButton(th, &v.browserUploadBtn, icons.FileFileUpload, components.IconPositionStart, "Upload")
					return btn.Layout(c5gtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(c5gtx layout.Context) layout.Dimensions {
					if !isObjectSelected {
						c5gtx = c5gtx.Disabled()
					}
					btn := components.SecondaryButton(th, &v.browserDownloadBtn, icons.FileFileDownload, components.IconPositionStart, "Download")
					return btn.Layout(c5gtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(c5gtx layout.Context) layout.Dimensions {
					if !isObjectSelected {
						c5gtx = c5gtx.Disabled()
					}
					btn := components.SecondaryButton(th, &v.browserDeleteBtn, icons.ActionDelete, components.IconPositionStart, "Delete")
					return btn.Layout(c5gtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(c5gtx layout.Context) layout.Dimensions {
					if !isObjectSelected {
						c5gtx = c5gtx.Disabled()
					}
					btn := components.SecondaryButton(th, &v.browserInfoBtn, icons.ActionInfo, components.IconPositionStart, "Info")
					return btn.Layout(c5gtx, th)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(c4gtx layout.Context) layout.Dimensions {
			if v.objectsLoading {
				return layout.Center.Layout(c4gtx, func(c6gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th.Material(), unit.Sp(14), "Loading objects...")
					lbl.Color = th.TextColor
					return lbl.Layout(c6gtx)
				})
			}

			if v.objectsError != "" {
				return layout.Center.Layout(c4gtx, func(c6gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th.Material(), unit.Sp(14), fmt.Sprintf("Error: %s", v.objectsError))
					lbl.Color = th.TextColor
					return lbl.Layout(c6gtx)
				})
			}

			return v.objectsList.Layout(c4gtx)
		}),
	)
}

func (v *ObjectsView) updateObjectsList(th *theme.Theme) {
	if v.objectsList == nil {
		return
	}

	// Only update if objects have changed (to preserve clickables)
	currentCount := len(v.objects)
	var currentHash string
	if currentCount > 0 {
		// Simple hash based on first and last object names + count
		currentHash = fmt.Sprintf("%d:%s:%s", currentCount, v.objects[0].Name, v.objects[currentCount-1].Name)
	} else {
		currentHash = "empty"
	}

	if currentCount == v.lastObjectsCount && currentHash == v.lastObjectsHash {
		// Objects haven't changed, just update selection
		v.objectsList.SetSelected(v.selectedObjectIdx)
		return
	}

	// Update tracking fields
	v.lastObjectsCount = currentCount
	v.lastObjectsHash = currentHash

	// Create new items
	items := make([]components.ListItem, len(v.objects))
	for i, obj := range v.objects {
		items[i] = components.ListItem{
			Title:    obj.Name,
			Subtitle: fmt.Sprintf("Size: %s | Modified: %s", v.formatBytes(obj.Size), obj.ModTime.Format(time.DateTime)),
		}
	}

	v.objectsList.SetItems(items)
	v.objectsList.SetSelected(v.selectedObjectIdx)
}

func (v *ObjectsView) layoutInfoModalContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if v.infoLoading {
		return layout.Center.Layout(gtx, func(c6gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Loading object info...")
			lbl.Color = th.TextColor
			return lbl.Layout(c6gtx)
		})
	}

	if v.infoResults == nil {
		return layout.Center.Layout(gtx, func(c6gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th.Material(), unit.Sp(14), "Failed to load object info")
			lbl.Color = th.TextColor
			return lbl.Layout(c6gtx)
		})
	}

	return v.layoutInfoContent(gtx, th)
}

func (v *ObjectsView) layoutInfoContent(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	info := v.infoResults
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Name", info.Name)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Size", v.formatBytes(info.Size))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Chunks", fmt.Sprintf("%d", info.Chunks))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "NUID", info.NUID)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Modified", info.ModTime.Format(time.DateTime))
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layoutDetailRow(cgtx, th, "Digest", info.Digest)
		}),
	)
}

// HandleShortcuts processes keyboard shortcuts for this view
// Returns true if a shortcut was handled
func (v *ObjectsView) HandleShortcuts(gtx layout.Context) bool {
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
func (v *ObjectsView) GetShortcutsHelp() []shortcuts.Shortcut {
	return []shortcuts.Shortcut{
		shortcuts.Create(func() {}),
		shortcuts.Refresh(func() {}),
		shortcuts.Delete(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.Browse(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.CopyName(func() bool { return v.SelectedIdx >= 0 }, func() {}),
		shortcuts.CopyRow(func() bool { return v.SelectedIdx >= 0 }, func() {}),
	}
}
