package components

import (
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type CheckBoxStyle struct {
	checkable
	CheckBox *widget.Bool
	theme    *theme.Theme
	onTab    func(gtx layout.Context, shift bool)
}

func CheckBox(th *material.Theme, checkBox *widget.Bool, label string) CheckBoxStyle {
	c := CheckBoxStyle{
		CheckBox: checkBox,
		checkable: checkable{
			Label:              label,
			Color:              th.Palette.Fg,
			IconColor:          th.Palette.Fg,
			TextSize:           th.TextSize * 12.0 / 14.0,
			Size:               24,
			shaper:             th.Shaper,
			checkedStateIcon:   th.Icon.CheckBoxChecked,
			uncheckedStateIcon: th.Icon.CheckBoxUnchecked,
		},
	}
	c.checkable.Font.Typeface = th.Face
	return c
}

func (c *CheckBoxStyle) SetTheme(th *theme.Theme) {
	c.theme = th
}

func (c *CheckBoxStyle) SetOnTab(fn func(gtx layout.Context, shift bool)) {
	c.onTab = fn
}

func (c *CheckBoxStyle) FocusTag() any {
	return c.CheckBox
}

// Layout updates the checkBox and displays it.
func (c CheckBoxStyle) Layout(gtx layout.Context) layout.Dimensions {
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: c.CheckBox, Name: key.NameSpace},
			key.Filter{Focus: c.CheckBox, Name: key.NameEnter},
			key.Filter{Focus: c.CheckBox, Name: key.NameReturn},
		)
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			c.CheckBox.Value = !c.CheckBox.Value
		}
	}

	// Handle TAB key for focus navigation
	if c.onTab != nil {
		for {
			ev, ok := gtx.Event(
				key.Filter{Focus: c.CheckBox, Name: key.NameTab, Optional: key.ModShift},
			)
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				c.onTab(gtx, ke.Modifiers.Contain(key.ModShift))
			}
		}
	}

	dims := c.CheckBox.Layout(gtx, func(gtx2 layout.Context) layout.Dimensions {
		semantic.CheckBox.Add(gtx2.Ops)
		return c.layout(gtx2, c.CheckBox.Value)
	})

	if gtx.Focused(c.CheckBox) && c.theme != nil {
		DrawFocusRing(gtx, c.theme.BorderColorFocused, dims.Size, gtx.Dp(unit.Dp(4)))
	}

	return dims
}
