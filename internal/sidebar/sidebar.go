package sidebar

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

var (
	hoverOverlayAlpha    uint8 = 48
	selectedOverlayAlpha uint8 = 96
)

type Sidebar struct {
	AlphaPalette

	selectedItem    int
	selectedChanged bool
	items           []renderItem

	itemList layout.List

	// pendingClick is set when a shortcut triggers a navigation
	// The click will be processed on the next layout frame
	pendingClick any // tag of item to click
}

func New() *Sidebar {
	m := &Sidebar{
		AlphaPalette: AlphaPalette{
			Hover:    hoverOverlayAlpha,
			Selected: selectedOverlayAlpha,
		},
	}
	return m
}

func (s *Sidebar) AddNavItem(item Item) {
	s.items = append(s.items, renderItem{
		Item:         item,
		AlphaPalette: &s.AlphaPalette,
	})
	if len(s.items) == 1 {
		s.items[0].selected = true
	}
}

func (s *Sidebar) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return s.LayoutContents(gtx, th)
}

func (s *Sidebar) LayoutContents(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	s.selectedChanged = false
	gtx.Constraints.Min.Y = 0
	s.itemList.Axis = layout.Vertical

	// Update internal navigation chains
	for i := range s.items {
		if i > 0 {
			s.items[i].prev = &s.items[i-1].Clickable
		}
		if i < len(s.items)-1 {
			s.items[i].next = &s.items[i+1].Clickable
		}
	}

	// Process pending click from keyboard shortcut
	if s.pendingClick != nil {
		for i := range s.items {
			if s.items[i].Tag == s.pendingClick && !s.items[i].Disabled {
				// Directly change selection without simulating button click
				// to avoid visual flash
				s.changeSelected(i)
				break
			}
		}
		s.pendingClick = nil
	}

	var clickedIndex int = -1

	dim := s.itemList.Layout(gtx, len(s.items), func(gtx C, index int) D {
		gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(70))
		gtx.Constraints.Min = gtx.Constraints.Max
		if s.items[index].Clicked(gtx) {
			clickedIndex = index
		}
		dim := s.items[index].Layout(gtx, th)
		return dim
	})

	if clickedIndex >= 0 {
		s.changeSelected(clickedIndex)
		gtx.Execute(op.InvalidateCmd{})
	}

	return dim
}

func (s *Sidebar) changeSelected(newIndex int) {
	if newIndex == s.selectedItem && s.items[s.selectedItem].selected {
		return
	}
	s.items[s.selectedItem].selected = false
	s.selectedItem = newIndex
	s.items[s.selectedItem].selected = true
	s.selectedChanged = true
}

func (s *Sidebar) SetSelected(tag interface{}) {
	for i, item := range s.items {
		if item.Tag == tag {
			s.changeSelected(i)
			break
		}
	}
}

// TriggerClick simulates a click on the item with the given tag.
// The click will be processed on the next layout frame.
func (s *Sidebar) TriggerClick(tag interface{}) {
	s.pendingClick = tag
}

// TriggerClickByIndex simulates a click on the item at the given index (1-based).
// Returns true if the index is valid and the item is not disabled.
func (s *Sidebar) TriggerClickByIndex(index int) bool {
	if index < 1 || index > len(s.items) {
		return false
	}
	if s.items[index-1].Disabled {
		return false
	}
	s.pendingClick = s.items[index-1].Tag
	return true
}

// GetItemCount returns the number of items in the sidebar
func (s *Sidebar) GetItemCount() int {
	return len(s.items)
}

func (s *Sidebar) Current() interface{} {
	return s.items[s.selectedItem].Tag
}

func (s *Sidebar) Changed() bool {
	return s.selectedChanged
}

func (s *Sidebar) FirstFocusTag() any {
	if len(s.items) == 0 {
		return nil
	}
	return &s.items[0].Clickable
}

func (s *Sidebar) LastFocusTag() any {
	if len(s.items) == 0 {
		return nil
	}
	return &s.items[len(s.items)-1].Clickable
}

func (s *Sidebar) SetNavigationLinks(next, prev any) {
	if len(s.items) == 0 {
		return
	}
	s.items[0].prev = prev
	s.items[len(s.items)-1].next = next
}

func (s *Sidebar) SetItemDisabled(tag any, disabled bool) {
	for i := range s.items {
		if s.items[i].Tag == tag {
			s.items[i].Item.Disabled = disabled
			break
		}
	}
}

func (s *Sidebar) IsItemDisabled(tag any) bool {
	for i := range s.items {
		if s.items[i].Tag == tag {
			return s.items[i].Item.Disabled
		}
	}
	return false
}
