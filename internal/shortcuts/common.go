package shortcuts

import "gioui.org/io/key"

// Save creates a save shortcut (Cmd+S)
func Save(action func()) Shortcut {
	return Shortcut{
		Name:      "Save",
		Key:       key.Name("S"),
		Modifiers: key.ModCommand,
		Action:    action,
		HelpText:  "Save current form/modal",
	}
}

// Refresh creates a refresh shortcut (Cmd+R)
func Refresh(action func()) Shortcut {
	return Shortcut{
		Name:      "Refresh",
		Key:       key.Name("R"),
		Modifiers: key.ModCommand,
		Action:    action,
		HelpText:  "Refresh current view",
	}
}

// Create creates a create/new shortcut (Cmd+N)
func Create(action func()) Shortcut {
	return Shortcut{
		Name:      "Create",
		Key:       key.Name("N"),
		Modifiers: key.ModCommand,
		Action:    action,
		HelpText:  "Create new item",
	}
}

// Delete creates a delete shortcut (Delete key)
func Delete(condition func() bool, action func()) Shortcut {
	return Shortcut{
		Name:      "Delete",
		Key:       key.NameDeleteForward,
		Modifiers: 0,
		Condition: condition,
		Action:    action,
		HelpText:  "Delete selected item",
	}
}

// DeleteBackward creates a delete shortcut (Backspace key)
func DeleteBackward(condition func() bool, action func()) Shortcut {
	return Shortcut{
		Name:      "Delete",
		Key:       key.NameDeleteBackward,
		Modifiers: 0,
		Condition: condition,
		Action:    action,
		HelpText:  "Delete selected item",
	}
}

// CloseModal creates a close modal shortcut (Escape)
func CloseModal(action func()) Shortcut {
	return Shortcut{
		Name:      "Close Modal",
		Key:       key.NameEscape,
		Modifiers: 0,
		Action:    action,
		HelpText:  "Close current modal",
	}
}

// Confirm creates a confirm/submit shortcut (Enter/Return when button focused)
func Confirm(action func()) Shortcut {
	return Shortcut{
		Name:      "Confirm",
		Key:       key.NameReturn,
		Modifiers: 0,
		Action:    action,
		HelpText:  "Confirm action",
	}
}

// ConfirmEnter creates a confirm shortcut using Enter key
func ConfirmEnter(action func()) Shortcut {
	return Shortcut{
		Name:      "Confirm",
		Key:       key.NameEnter,
		Modifiers: 0,
		Action:    action,
		HelpText:  "Confirm action",
	}
}

// NavigateTo creates a navigation shortcut
func NavigateTo(name string, keyName key.Name, action func()) Shortcut {
	return Shortcut{
		Name:      "Navigate to " + name,
		Key:       keyName,
		Modifiers: key.ModCommand,
		Action:    action,
		HelpText:  "Navigate to " + name,
	}
}

// GlobalSearch creates the global search shortcut (Cmd+K)
func GlobalSearch(action func()) Shortcut {
	return Shortcut{
		Name:      "Global Search",
		Key:       key.Name("K"),
		Modifiers: key.ModCommand,
		Action:    action,
		HelpText:  "Open global search",
	}
}

// ShowHelp creates the help shortcut (Cmd+H)
func ShowHelp(action func()) Shortcut {
	return Shortcut{
		Name:      "Show Help",
		Key:       key.Name("H"),
		Modifiers: key.ModCommand,
		Action:    action,
		HelpText:  "Show keyboard shortcuts help",
	}
}

// Preferences creates the preferences shortcut (Cmd+,)
func Preferences(action func()) Shortcut {
	return Shortcut{
		Name:      "Preferences",
		Key:       key.Name(","),
		Modifiers: key.ModCommand,
		Action:    action,
		HelpText:  "Open preferences",
	}
}

// Edit creates an edit shortcut (Cmd+E)
func Edit(condition func() bool, action func()) Shortcut {
	return Shortcut{
		Name:      "Edit",
		Key:       key.Name("E"),
		Modifiers: key.ModCommand,
		Condition: condition,
		Action:    action,
		HelpText:  "Edit selected item",
	}
}

// Browse creates a browse shortcut (Cmd+B)
func Browse(condition func() bool, action func()) Shortcut {
	return Shortcut{
		Name:      "Browse",
		Key:       key.Name("B"),
		Modifiers: key.ModCommand,
		Condition: condition,
		Action:    action,
		HelpText:  "Browse selected item",
	}
}

// Import creates an import shortcut (Cmd+I)
func Import(action func()) Shortcut {
	return Shortcut{
		Name:      "Import",
		Key:       key.Name("I"),
		Modifiers: key.ModCommand,
		Action:    action,
		HelpText:  "Import items",
	}
}

// Export creates an export shortcut (Cmd+Shift+E)
func Export(action func()) Shortcut {
	return Shortcut{
		Name:      "Export",
		Key:       key.Name("E"),
		Modifiers: key.ModCommand | key.ModShift,
		Action:    action,
		HelpText:  "Export items",
	}
}

// Connect creates a connect shortcut (Enter/Return)
func Connect(condition func() bool, action func()) Shortcut {
	return Shortcut{
		Name:      "Connect",
		Key:       key.NameReturn,
		Modifiers: 0,
		Condition: condition,
		Action:    action,
		HelpText:  "Connect to selected item",
	}
}

// Toggle creates a toggle shortcut with custom key
func Toggle(name string, keyName key.Name, modifiers key.Modifiers, condition func() bool, action func()) Shortcut {
	return Shortcut{
		Name:      name,
		Key:       keyName,
		Modifiers: modifiers,
		Condition: condition,
		Action:    action,
		HelpText:  "Toggle " + name,
	}
}

// Custom creates a fully custom shortcut
func Custom(name, helpText string, keyName key.Name, modifiers key.Modifiers, condition func() bool, action func()) Shortcut {
	return Shortcut{
		Name:      name,
		Key:       keyName,
		Modifiers: modifiers,
		Condition: condition,
		Action:    action,
		HelpText:  helpText,
	}
}
