package shortcuts

import "gioui.org/io/key"

func CopyName(condition func() bool, action func()) Shortcut {
	return Shortcut{
		Name:      "Copy Name",
		Key:       key.Name("C"),
		Modifiers: key.ModCtrl,
		Condition: condition,
		Action:    action,
		HelpText:  "Copy selected item name",
	}
}

func CopyRow(condition func() bool, action func()) Shortcut {
	return Shortcut{
		Name:      "Copy Row",
		Key:       key.Name("C"),
		Modifiers: key.ModCtrl | key.ModShift,
		Condition: condition,
		Action:    action,
		HelpText:  "Copy entire row as tab-separated values",
	}
}
