package shortcuts

import (
	"gioui.org/io/key"
)

// ModalChecker is an interface for checking if a modal is visible
type ModalChecker interface {
	IsVisible() bool
}

// IsAnyModalVisible checks if any of the provided modals are visible
// Usage: IsAnyModalVisible(v.addModal, v.editModal, v.removeModal)
func IsAnyModalVisible(modals ...interface{ IsVisible() bool }) bool {
	for _, m := range modals {
		if m.IsVisible() {
			return true
		}
	}
	return false
}

// CheckModalsWithFormModal checks visibility including FormModal type
func CheckModalsWithFormModal(modals ...interface{}) bool {
	for _, m := range modals {
		switch modal := m.(type) {
		case interface{ IsVisible() bool }:
			if modal.IsVisible() {
				return true
			}
		case interface{ GetVisible() bool }:
			if modal.GetVisible() {
				return true
			}
		}
	}
	return false
}

// FormatShortcut returns a human-readable representation of a shortcut
func FormatShortcut(sc Shortcut) string {
	var mods string
	if sc.Modifiers&key.ModCommand != 0 {
		mods += "Cmd+"
	}
	if sc.Modifiers&key.ModCtrl != 0 {
		mods += "Ctrl+"
	}
	if sc.Modifiers&key.ModAlt != 0 {
		mods += "Alt+"
	}
	if sc.Modifiers&key.ModShift != 0 {
		mods += "Shift+"
	}

	keyName := string(sc.Key)
	// Map special keys to readable names
	switch sc.Key {
	case key.NameReturn:
		keyName = "Enter"
	case key.NameEnter:
		keyName = "Enter"
	case key.NameEscape:
		keyName = "Esc"
	case key.NameDeleteForward:
		keyName = "Delete"
	case key.NameDeleteBackward:
		keyName = "Backspace"
	case key.NameUpArrow:
		keyName = "↑"
	case key.NameDownArrow:
		keyName = "↓"
	case key.NameLeftArrow:
		keyName = "←"
	case key.NameRightArrow:
		keyName = "→"
	}

	return mods + keyName
}

// GroupShortcutsByCategory groups shortcuts for help display
type ShortcutGroup struct {
	Title     string
	Shortcuts []Shortcut
}

// GroupShortcuts organizes shortcuts into logical groups
func GroupShortcuts(shortcuts []Shortcut) []ShortcutGroup {
	groups := make(map[string][]Shortcut)

	for _, sc := range shortcuts {
		// Determine group based on shortcut characteristics
		group := "General"
		switch {
		case sc.Modifiers == key.ModCommand && (sc.Key == key.Name("1") || sc.Key == key.Name("2") ||
			sc.Key == key.Name("3") || sc.Key == key.Name("4") || sc.Key == key.Name("5") ||
			sc.Key == key.Name("6") || sc.Key == key.Name("7") || sc.Key == key.Name("8") ||
			sc.Key == key.Name("9")):
			group = "Navigation"
		case sc.Name == "Create" || sc.Name == "Delete" || sc.Name == "Edit" || sc.Name == "Copy Name" || sc.Name == "Copy Row":
			group = "Actions"
		case sc.Name == "Save" || sc.Name == "Refresh":
			group = "General"
		case sc.Name == "Close Modal":
			group = "Modal"
		}

		groups[group] = append(groups[group], sc)
	}

	// Convert map to slice with consistent ordering
	var result []ShortcutGroup
	order := []string{"Navigation", "Actions", "General", "Modal"}
	for _, title := range order {
		if shortcuts, ok := groups[title]; ok {
			result = append(result, ShortcutGroup{
				Title:     title,
				Shortcuts: shortcuts,
			})
		}
	}

	// Add any remaining groups
	for title, shortcuts := range groups {
		found := false
		for _, g := range result {
			if g.Title == title {
				found = true
				break
			}
		}
		if !found {
			result = append(result, ShortcutGroup{
				Title:     title,
				Shortcuts: shortcuts,
			})
		}
	}

	return result
}

// FilterActive returns only shortcuts that can be triggered (condition met)
func FilterActive(shortcuts []Shortcut) []Shortcut {
	var active []Shortcut
	for _, sc := range shortcuts {
		if sc.Condition == nil || sc.Condition() {
			active = append(active, sc)
		}
	}
	return active
}
