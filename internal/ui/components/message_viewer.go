package components

import (
	"image"
	"image/color"
	"strings"
	"sync/atomic"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"

	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

// MessageViewerItem represents an item that can be displayed in the MessageViewerModal
type MessageViewerItem struct {
	ID       string
	Title    string
	Subtitle string
	Content  string
	Format   string // "JSON", "XML", "Text", etc.
	Deleted  bool
	Created  time.Time
	Metadata map[string]string
	Icon     *widget.Icon
}

// MessageViewerModal is a reusable modal component for viewing messages/items
// with a list on the left and content viewer on the right
type MessageViewerModal struct {
	// State
	IsOpen         bool
	Title          string
	Items          []MessageViewerItem
	FilteredItems  []MessageViewerItem
	SelectedIndex  int
	SelectedItemID string

	// Loading state for async content loading
	IsLoading      bool
	LoadingMessage string

	// Content display
	content      string
	lastLoadedID string
	loadID       int32
	codeEditor   *CodeEditor

	// UI Components
	listWidget   *ListStyle
	searchEditor *InputField
	splitView    SplitView
	closeBtn     widget.Clickable
	backdrop     widget.Clickable
	addBtn       widget.Clickable

	// Filters
	showDeleted         bool
	showDeletedBool     widget.Bool
	showDeletedCheckbox bool          // Whether to show the "Show Deleted" checkbox
	showDeletedCB       CheckBoxStyle // Stored checkbox for tab navigation

	// Focus management
	focusInit      bool
	focusIndex     int
	lastFocusedTag any // Track last focused element to detect focus changes

	// Actions
	onDelete      func(item MessageViewerItem)
	onPurge       func(item MessageViewerItem)
	onEdit        func(item MessageViewerItem)        // For KV edit view
	onAdd         func()                              // For adding new items (KV Add Key)
	onLoadContent func(item MessageViewerItem) string // Called to load content for selected item
	onClose       func()

	// External modal check - function to check if confirmation modal is visible
	IsConfirmationModalVisible func() bool

	// Invalidation callback - called when UI needs to be redrawn
	onInvalidate func()

	// Infinite scroll / load more
	onLoadMore      func()  // Callback when user scrolls to bottom
	isLoadingMore   bool    // Whether we're currently loading more items
	hasMoreItems    bool    // Whether there are more items to load
	loadMoreOffset  int     // Number of items from bottom to trigger load
	scrollThreshold float32 // Scroll position threshold (0-1) to trigger load

	// Theme
	th *theme.Theme
}

// NewMessageViewerModal creates a new MessageViewerModal instance
func NewMessageViewerModal(th *theme.Theme) *MessageViewerModal {
	m := &MessageViewerModal{
		th:            th,
		listWidget:    NewList(th),
		codeEditor:    NewCodeEditor("", CodeLanguageText, th),
		SelectedIndex: -1,
		searchEditor:  NewInputField("Filter items..."),
		splitView: SplitView{
			Resize:   component.Resize{Ratio: 0.4},
			BarWidth: unit.Dp(2),
		},
	}

	// Set up list widget callbacks
	m.listWidget.OnSelect(func(idx int) {
		m.SelectedIndex = idx
		if idx >= 0 && idx < len(m.FilteredItems) {
			item := m.FilteredItems[idx]
			m.SelectedItemID = item.ID
			m.loadContent(item)
		}
		m.invalidate()
	})

	m.listWidget.SetFocusTag(m.listWidget)

	return m
}

// SetActions configures the delete, purge, and optional edit action handlers
func (m *MessageViewerModal) SetActions(onDelete, onPurge, onEdit func(item MessageViewerItem)) {
	m.onDelete = onDelete
	m.onPurge = onPurge
	m.onEdit = onEdit
}

// SetOnDoubleClick sets the callback for double-clicking an item in the list
func (m *MessageViewerModal) SetOnDoubleClick(fn func(item MessageViewerItem)) {
	// Wire it up with the list widget's OnDoubleClick
	m.listWidget.OnDoubleClick(func(idx int) {
		if idx >= 0 && idx < len(m.FilteredItems) {
			fn(m.FilteredItems[idx])
		}
	})
}

// setupActions configures the action buttons on the list widget
func (m *MessageViewerModal) setupActions() {
	// Add edit action (if available)
	if m.onEdit != nil {
		m.listWidget.AddAction(icons.EditorModeEdit, "Edit", func(idx int) {
			if idx >= 0 && idx < len(m.FilteredItems) {
				m.onEdit(m.FilteredItems[idx])
			}
		})
	}

	// Add delete action
	if m.onDelete != nil {
		m.listWidget.AddAction(icons.ActionDelete, "Delete", func(idx int) {
			if idx >= 0 && idx < len(m.FilteredItems) {
				m.onDelete(m.FilteredItems[idx])
			}
		})
	}

	// Add purge action
	if m.onPurge != nil {
		m.listWidget.AddAction(icons.ContentDeleteSweep, "Purge", func(idx int) {
			if idx >= 0 && idx < len(m.FilteredItems) {
				m.onPurge(m.FilteredItems[idx])
			}
		})
	}
}

// SetOnLoadContent sets the callback for loading item content
func (m *MessageViewerModal) SetOnLoadContent(fn func(item MessageViewerItem) string) {
	m.onLoadContent = fn
}

// SetOnClose sets the callback for when the modal is closed
func (m *MessageViewerModal) SetOnClose(fn func()) {
	m.onClose = fn
}

// SetAddAction sets the callback for the Add button (shown in header)
func (m *MessageViewerModal) SetAddAction(onAdd func()) {
	m.onAdd = onAdd
}

// SetOnInvalidate sets the callback for when the UI needs to be redrawn
func (m *MessageViewerModal) SetOnInvalidate(fn func()) {
	m.onInvalidate = fn
}

// invalidate triggers a UI redraw
func (m *MessageViewerModal) invalidate() {
	if m.onInvalidate != nil {
		m.onInvalidate()
	}
}

// FocusTag returns the focus tag for the modal's list widget
func (m *MessageViewerModal) FocusTag() any {
	return m.listWidget.FocusTag()
}

// Open opens the modal with the given items
func (m *MessageViewerModal) Open(title string, items []MessageViewerItem) {
	m.IsOpen = true
	m.Title = title
	m.Items = items
	m.SelectedIndex = -1
	m.SelectedItemID = ""
	m.content = ""
	m.lastLoadedID = ""
	m.focusInit = false
	m.focusIndex = 0
	m.searchEditor.SetText("")
	m.showDeleted = false
	m.showDeletedBool.Value = false

	// Setup actions only once (on first open)
	if len(m.listWidget.actions) == 0 {
		m.setupActions()
	}

	// Apply initial filter (hide deleted items by default)
	m.filterItems()
}

// SetShowDeletedCheckbox sets whether to show the "Show Deleted" checkbox
func (m *MessageViewerModal) SetShowDeletedCheckbox(show bool) {
	m.showDeletedCheckbox = show
}

// SetOnLoadMore sets the callback for loading more items when scrolling
func (m *MessageViewerModal) SetOnLoadMore(callback func()) {
	m.onLoadMore = callback
}

// SetHasMoreItems sets whether there are more items available to load
func (m *MessageViewerModal) SetHasMoreItems(hasMore bool) {
	m.hasMoreItems = hasMore
}

// SetLoadingMore sets the loading more state
func (m *MessageViewerModal) SetLoadingMore(loading bool) {
	m.isLoadingMore = loading
}

// Close closes the modal
func (m *MessageViewerModal) Close() {
	m.IsOpen = false
	m.focusInit = false
	m.focusIndex = 0
	m.lastFocusedTag = nil
	m.SelectedIndex = -1
	m.SelectedItemID = ""
	if m.onClose != nil {
		m.onClose()
	}
}

// UpdateItems updates the items in the modal
func (m *MessageViewerModal) UpdateItems(items []MessageViewerItem) {
	m.Items = items
	m.filterItems()
}

// filterItems filters items based on search text and showDeleted flag
func (m *MessageViewerModal) filterItems() {
	query := strings.ToLower(m.searchEditor.GetText())
	m.FilteredItems = make([]MessageViewerItem, 0)

	newSelectedIdx := -1
	for _, item := range m.Items {
		// Skip deleted items if not showing them
		if item.Deleted && !m.showDeleted {
			continue
		}

		// Apply search filter
		if query == "" || strings.Contains(strings.ToLower(item.Title), query) ||
			strings.Contains(strings.ToLower(item.Subtitle), query) {
			if m.SelectedItemID != "" && item.ID == m.SelectedItemID {
				newSelectedIdx = len(m.FilteredItems)
			}
			m.FilteredItems = append(m.FilteredItems, item)
		}
	}

	m.SelectedIndex = newSelectedIdx
	if m.SelectedIndex >= 0 && m.SelectedIndex < len(m.FilteredItems) {
		m.loadContent(m.FilteredItems[m.SelectedIndex])
	}

	m.updateListItems()
}

// updateListItems updates the list widget with current filtered items
func (m *MessageViewerModal) updateListItems() {
	listItems := make([]ListItem, len(m.FilteredItems))
	for i, item := range m.FilteredItems {
		listItems[i] = ListItem{
			Title:    item.Title,
			Subtitle: item.Subtitle,
			Icon:     item.Icon,
			Deleted:  item.Deleted,
			Created:  item.Created,
		}
	}
	m.listWidget.SetItems(listItems)
	m.listWidget.SetSelected(m.SelectedIndex)
}

// loadContent loads the content for the given item
func (m *MessageViewerModal) loadContent(item MessageViewerItem) {
	if m.onLoadContent != nil {
		id := atomic.AddInt32(&m.loadID, 1)
		go func() {
			content := m.onLoadContent(item)
			if id == atomic.LoadInt32(&m.loadID) {
				m.content = content
				m.updateCodeEditorLanguage(item.Format)
				m.invalidate()
			}
		}()
	} else {
		m.content = item.Content
		m.updateCodeEditorLanguage(item.Format)
		m.invalidate()
	}
}

// updateCodeEditorLanguage updates the code editor language based on format
func (m *MessageViewerModal) updateCodeEditorLanguage(format string) {
	lang := CodeLanguageText
	switch format {
	case "JSON":
		lang = CodeLanguageJSON
	case "XML":
		lang = CodeLanguageXML
	}
	m.codeEditor.SetLanguage(lang)
}

// Layout renders the modal
func (m *MessageViewerModal) Layout(gtx layout.Context) layout.Dimensions {
	if !m.IsOpen {
		return layout.Dimensions{}
	}

	// Register for events - use clip area to capture all events within modal bounds
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, m)
	area.Pop()

	// Handle Tab navigation at modal level - capture TAB regardless of focus
	// This works because we process it before children lay out
	// But skip if a confirmation modal is visible (blocks focus from escaping)
	if m.IsConfirmationModalVisible == nil || !m.IsConfirmationModalVisible() {
		for {
			ev, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press {
				m.handleTab(gtx, e.Modifiers.Contain(key.ModShift))
			}
		}
	}

	// Initialize focus on first render
	if !m.focusInit {
		m.focusInit = true
		m.focusIndex = 0
		gtx.Execute(key.FocusCmd{Tag: m.searchEditor.FocusTag()})
	}

	// Check if focus moved to the list widget and auto-select first item if needed
	listFocusTag := m.listWidget.FocusTag()
	if gtx.Source.Focused(listFocusTag) && m.lastFocusedTag != listFocusTag {
		// Focus just moved to the list widget
		if m.SelectedIndex == -1 && len(m.FilteredItems) > 0 {
			// No previous selection and items exist - select the first one
			m.SelectedIndex = 0
			m.SelectedItemID = m.FilteredItems[0].ID
			m.listWidget.SetSelected(0)
			m.loadContent(m.FilteredItems[0])
		}
	}
	// Update last focused tag
	if gtx.Source.Focused(listFocusTag) {
		m.lastFocusedTag = listFocusTag
	} else if gtx.Source.Focused(m.searchEditor.FocusTag()) {
		m.lastFocusedTag = m.searchEditor.FocusTag()
	} else if gtx.Source.Focused(m.codeEditor.FocusTag()) {
		m.lastFocusedTag = m.codeEditor.FocusTag()
	} else if m.showDeletedCheckbox && gtx.Source.Focused(m.showDeletedCB.FocusTag()) {
		m.lastFocusedTag = m.showDeletedCB.FocusTag()
	}

	// Handle close button
	for m.closeBtn.Clicked(gtx) {
		m.Close()
	}

	// Handle add button
	if m.onAdd != nil {
		for m.addBtn.Clicked(gtx) {
			m.onAdd()
		}
	}

	// Handle backdrop click
	for m.backdrop.Clicked(gtx) {
		m.Close()
	}

	// Handle show deleted toggle
	if m.showDeletedBool.Update(gtx) {
		m.showDeleted = m.showDeletedBool.Value
		m.filterItems()
	}

	// Handle search filter changes
	if m.searchEditor.Changed() {
		m.filterItems()
	}

	// Handle Escape key - only if no confirmation modal is visible
	if m.IsConfirmationModalVisible == nil || !m.IsConfirmationModalVisible() {
		for {
			ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press {
				m.Close()
			}
		}
	}

	// Handle list keyboard shortcuts (Delete, Shift+Delete, Enter)
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: m.listWidget.FocusTag(), Name: key.NameDeleteForward, Optional: key.ModShift},
			key.Filter{Focus: m.listWidget.FocusTag(), Name: key.NameDeleteBackward, Optional: key.ModShift},
		)
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			if m.SelectedIndex >= 0 && m.SelectedIndex < len(m.FilteredItems) {
				if e.Modifiers.Contain(key.ModShift) {
					if m.onPurge != nil {
						m.onPurge(m.FilteredItems[m.SelectedIndex])
					}
				} else {
					if m.onDelete != nil {
						m.onDelete(m.FilteredItems[m.SelectedIndex])
					}
				}
			}
		}
	}

	// Handle Enter key on list - trigger edit action
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: m.listWidget.FocusTag(), Name: key.NameEnter},
			key.Filter{Focus: m.listWidget.FocusTag(), Name: key.NameReturn},
		)
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			if m.SelectedIndex >= 0 && m.SelectedIndex < len(m.FilteredItems) {
				item := m.FilteredItems[m.SelectedIndex]
				if m.onEdit != nil && !item.Deleted {
					m.onEdit(item)
				}
			}
		}
	}

	// Render backdrop
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 150}, clip.Rect{Max: gtx.Constraints.Max}.Op())

	// Layout backdrop click areas
	m.layoutBackdrop(gtx)

	// Update code editor content if changed
	if m.content != m.lastLoadedID {
		m.codeEditor.SetCode(m.content)
		m.lastLoadedID = m.content
	}

	// Layout modal content
	return layout.Center.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Inset{
			Top:    unit.Dp(40),
			Bottom: unit.Dp(40),
			Left:   unit.Dp(60),
			Right:  unit.Dp(60),
		}.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
			ccgtx.Constraints.Min = ccgtx.Constraints.Max
			return Card{}.Layout(ccgtx, m.th, func(cccgtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
					// Header
					layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
						return m.layoutHeader(c4gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(1)}.Layout),
					layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
						paint.FillShape(c4gtx.Ops, m.th.TableBorderColor, clip.Rect{Max: image.Pt(c4gtx.Constraints.Max.X, c4gtx.Dp(1))}.Op())
						return layout.Dimensions{Size: image.Pt(c4gtx.Constraints.Max.X, c4gtx.Dp(1))}
					}),
					// Filter bar
					layout.Rigid(func(c4gtx layout.Context) layout.Dimensions {
						return m.layoutFilterBar(c4gtx)
					}),
					// Content
					layout.Flexed(1, func(c4gtx layout.Context) layout.Dimensions {
						return m.layoutContent(c4gtx)
					}),
				)
			})
		})
	})
}

// layoutBackdrop renders the clickable backdrop areas around the modal
func (m *MessageViewerModal) layoutBackdrop(gtx layout.Context) {
	// Top gutter
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return m.backdrop.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(ccgtx.Constraints.Max.X, ccgtx.Dp(unit.Dp(40)))}
			})
		}),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					// Left gutter
					return m.backdrop.Layout(ccgtx, func(cccgtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(cccgtx.Dp(unit.Dp(60)), cccgtx.Constraints.Max.Y)}
					})
				}),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					// Right gutter
					return m.backdrop.Layout(ccgtx, func(cccgtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(cccgtx.Dp(unit.Dp(60)), cccgtx.Constraints.Max.Y)}
					})
				}),
			)
		}),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			// Bottom gutter
			return m.backdrop.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(ccgtx.Constraints.Max.X, ccgtx.Dp(unit.Dp(40)))}
			})
		}),
	)
}

// layoutHeader renders the modal header
func (m *MessageViewerModal) layoutHeader(gtx layout.Context) layout.Dimensions {
	children := []layout.FlexChild{
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(16)}.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				lbl := material.Label(m.th.Material(), unit.Sp(18), m.Title)
				lbl.Color = m.th.TextColor
				return lbl.Layout(ccgtx)
			})
		}),
	}

	// Add button (if onAdd is set)
	if m.onAdd != nil {
		children = append(children,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(8)}.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
					btn := SecondaryButton(m.th, &m.addBtn, icons.ContentAddCircle, IconPositionStart, "Add")
					return btn.Layout(ccgtx, m.th)
				})
			}),
		)
	}

	// Close button
	children = append(children,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(16)}.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				btn := IconButton{
					Icon:      icons.NavigationClose,
					Clickable: &m.closeBtn,
					Size:      unit.Dp(24),
					Color:     m.th.TextColor,
				}
				return btn.Layout(ccgtx, m.th)
			})
		}),
	)

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

// layoutFilterBar renders the search filter and options
func (m *MessageViewerModal) layoutFilterBar(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				ccgtx.Constraints.Max.X = ccgtx.Dp(unit.Dp(300))
				return m.searchEditor.Layout(ccgtx, m.th)
			}),
		}

		// Only show "Show Deleted" checkbox if enabled
		if m.showDeletedCheckbox {
			children = append(children,
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					m.showDeletedCB = CheckBox(m.th.Material(), &m.showDeletedBool, "Show Deleted")
					m.showDeletedCB.SetTheme(m.th)
					m.showDeletedCB.SetOnTab(func(gtx layout.Context, shift bool) {
						m.handleTab(gtx, shift)
					})
					return m.showDeletedCB.Layout(ccgtx)
				}),
			)
		}

		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(cgtx, children...)
	})
}

// layoutContent renders the split view with list and content
func (m *MessageViewerModal) layoutContent(gtx layout.Context) layout.Dimensions {
	dims := m.splitView.Layout(gtx, m.th,
		func(cgtx layout.Context) layout.Dimensions {
			// List of items
			listDims := m.listWidget.Layout(cgtx)

			// Check if we need to load more items (infinite scroll)
			if m.onLoadMore != nil && m.hasMoreItems && !m.isLoadingMore {
				// Trigger load when near the end (within last 5 items)
				if m.listWidget.IsNearEnd(5) {
					m.isLoadingMore = true
					go func() {
						m.onLoadMore()
						m.isLoadingMore = false
					}()
				}
			}

			return listDims
		},
		func(cgtx layout.Context) layout.Dimensions {
			// Content viewer
			if m.SelectedIndex == -1 {
				return layout.Center.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
					lbl := material.Label(m.th.Material(), unit.Sp(14), "Select an item to view content")
					lbl.Color = m.th.TextColor
					return lbl.Layout(ccgtx)
				})
			}

			return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
				layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
					return m.codeEditor.Layout(ccgtx, m.th)
				}),
			)
		},
	)

	return dims
}

// handleTab handles tab navigation within the modal
func (m *MessageViewerModal) handleTab(gtx layout.Context, shift bool) {
	// Build list of focusable tags in visual order (top to bottom, left to right)
	var tags []any

	// Add button (if present) - top row
	if m.onAdd != nil {
		tags = append(tags, &m.addBtn)
	}

	// Close button - top row
	tags = append(tags, &m.closeBtn)

	// Search editor - filter bar
	tags = append(tags, m.searchEditor.FocusTag())

	// Include checkbox in tab order if it's visible
	if m.showDeletedCheckbox {
		tags = append(tags, m.showDeletedCB.FocusTag())
	}

	// List and code editor - content area
	tags = append(tags,
		m.listWidget.FocusTag(),
		m.codeEditor.FocusTag(),
	)

	curIdx := -1
	for i, tag := range tags {
		if gtx.Source.Focused(tag) {
			curIdx = i
			break
		}
	}

	if curIdx == -1 {
		if len(tags) > 0 {
			gtx.Execute(key.FocusCmd{Tag: tags[0]})
		}
	} else {
		if shift {
			nextIdx := (curIdx - 1 + len(tags)) % len(tags)
			gtx.Execute(key.FocusCmd{Tag: tags[nextIdx]})
		} else {
			nextIdx := (curIdx + 1) % len(tags)
			gtx.Execute(key.FocusCmd{Tag: tags[nextIdx]})
		}
	}
}
