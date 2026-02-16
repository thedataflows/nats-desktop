package shortcuts

import (
	"gioui.org/io/key"
	"gioui.org/layout"
)

// Shortcut represents a keyboard shortcut with optional condition
type Shortcut struct {
	Name      string
	Key       key.Name
	Modifiers key.Modifiers
	Condition func() bool // Optional predicate - shortcut only works if this returns true
	Action    func()
	HelpText  string // For help display
}

// Handler manages a collection of shortcuts
type Handler struct {
	shortcuts []Shortcut
}

// NewHandler creates a new shortcut handler with the given shortcuts
func NewHandler(shortcuts []Shortcut) *Handler {
	return &Handler{
		shortcuts: shortcuts,
	}
}

// Handle processes a key event and executes matching shortcuts
// Returns true if a shortcut was handled
func (h *Handler) Handle(gtx layout.Context, event key.Event) bool {
	for _, sc := range h.shortcuts {
		if sc.Match(event) {
			if sc.Condition == nil || sc.Condition() {
				sc.Action()
				return true
			}
		}
	}
	return false
}

// Match checks if this shortcut matches the given key event
func (sc *Shortcut) Match(event key.Event) bool {
	// Match key name
	nameMatch := event.Name == sc.Key

	// Match modifiers (ignore NumLock, etc. by only checking known modifiers)
	mods := event.Modifiers & (key.ModCtrl | key.ModShift | key.ModAlt | key.ModCommand)

	return nameMatch && mods == sc.Modifiers && event.State == key.Press
}

// GetHelp returns all shortcuts for help display
func (h *Handler) GetHelp() []Shortcut {
	return h.shortcuts
}

// Add adds a new shortcut to the handler
func (h *Handler) Add(sc Shortcut) {
	h.shortcuts = append(h.shortcuts, sc)
}

// Remove removes a shortcut by name
func (h *Handler) Remove(name string) {
	filtered := make([]Shortcut, 0, len(h.shortcuts))
	for _, sc := range h.shortcuts {
		if sc.Name != name {
			filtered = append(filtered, sc)
		}
	}
	h.shortcuts = filtered
}

// FilterEnabled returns only shortcuts whose conditions are met
func (h *Handler) FilterEnabled() []Shortcut {
	var enabled []Shortcut
	for _, sc := range h.shortcuts {
		if sc.Condition == nil || sc.Condition() {
			enabled = append(enabled, sc)
		}
	}
	return enabled
}
