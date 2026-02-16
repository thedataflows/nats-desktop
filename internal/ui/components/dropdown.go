package components

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type DropDown struct {
	menuContextArea component.ContextArea
	menu            component.MenuState
	list            *widget.List

	MinWidth      unit.Dp
	MaxWidth      unit.Dp
	MaxMenuHeight unit.Dp // Maximum height of the dropdown menu (0 = default 300)
	menuInit      bool

	isOpen              bool
	keyboardOpen        bool // Track if menu opened via keyboard
	selectedOptionIndex int
	lastSelectedIndex   int
	options             []*DropDownOption

	size image.Point

	borderWidth  unit.Dp
	cornerRadius unit.Dp

	changed       bool
	onValueChange func(value string)

	clickable widget.Clickable

	// Label support
	Label         string
	ShowLabel     bool
	LabelPosition LabelPosition
}

func (c *DropDown) FocusTag() any {
	return &c.clickable
}

type DropDownOption struct {
	Text       string
	Value      string
	Identifier string
	clickable  widget.Clickable

	Icon      *widget.Icon
	IconColor color.NRGBA
	IconSize  unit.Dp

	isDivider bool
	isDefault bool
}

func NewDropDownOption(text string) *DropDownOption {
	return &DropDownOption{
		Text:      text,
		isDivider: false,
	}
}

func NewDropDownDivider() *DropDownOption {
	return &DropDownOption{
		isDivider: true,
	}
}

func (o *DropDownOption) WithIdentifier(identifier string) *DropDownOption {
	o.Identifier = identifier
	return o
}

func (o *DropDownOption) WithValue(value string) *DropDownOption {
	o.Value = value
	return o
}

func (o *DropDownOption) WithIcon(icon *widget.Icon, color color.NRGBA, size unit.Dp) *DropDownOption {
	o.Icon = icon
	o.IconColor = color
	o.IconSize = size
	return o
}

func (o *DropDownOption) DefaultSelected() *DropDownOption {
	o.isDefault = true
	return o
}

func (o *DropDownOption) GetText() string {
	if o == nil {
		return ""
	}

	return o.Text
}

func (o *DropDownOption) GetValue() string {
	if o == nil {
		return ""
	}

	return o.Value
}

func (c *DropDown) SetSelected(index int) {
	c.selectedOptionIndex = index
	c.lastSelectedIndex = index
}

func (c *DropDown) SetOnChanged(f func(value string)) {
	c.onValueChange = f
}

func (c *DropDown) SetSelectedByTitle(title string) {
	if len(c.options) == 0 {
		return
	}

	for i, opt := range c.options {
		if opt.Text == title {
			c.selectedOptionIndex = i
			c.lastSelectedIndex = i
			break
		}
	}
}

func (c *DropDown) SetSelectedByIdentifier(identifier string) {
	for i, opt := range c.options {
		if opt.Identifier == identifier {
			c.selectedOptionIndex = i
			c.lastSelectedIndex = i
			break
		}
	}
}

func (c *DropDown) SetSelectedByValue(value string) {
	for i, opt := range c.options {
		if opt.Value == value {
			c.selectedOptionIndex = i
			c.lastSelectedIndex = i
			break
		}
	}
}

func NewDropDown(options ...*DropDownOption) *DropDown {
	c := &DropDown{
		menuContextArea: component.ContextArea{
			Activation:       pointer.ButtonPrimary,
			AbsolutePosition: true,
		},
		list: &widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
		options:      options,
		borderWidth:  unit.Dp(1),
		cornerRadius: unit.Dp(8),
		menuInit:     true,
	}

	return c
}

func NewDropDownWithoutBorder(options ...*DropDownOption) *DropDown {
	c := &DropDown{
		menuContextArea: component.ContextArea{
			Activation:       pointer.ButtonPrimary,
			AbsolutePosition: true,
		},
		list: &widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
		options:  options,
		menuInit: true,
	}

	return c
}

// NewLabeledDropDown creates a new dropdown with a label displayed on top
func NewLabeledDropDown(label string, options ...*DropDownOption) *DropDown {
	c := NewDropDown(options...)
	c.Label = label
	c.ShowLabel = true
	c.LabelPosition = LabelPositionTop
	return c
}

// NewLabeledDropDownWithPosition creates a new dropdown with a label in the specified position
func NewLabeledDropDownWithPosition(label string, pos LabelPosition, options ...*DropDownOption) *DropDown {
	c := NewDropDown(options...)
	c.Label = label
	c.ShowLabel = true
	c.LabelPosition = pos
	return c
}

// SetLabelPosition sets the position of the label (Top or Left)
func (c *DropDown) SetLabelPosition(pos LabelPosition) {
	c.LabelPosition = pos
}

func (c *DropDown) SelectedIndex() int {
	return c.selectedOptionIndex
}

func (c *DropDown) SetOptions(options ...*DropDownOption) {
	c.selectedOptionIndex = 0
	c.options = options
	if len(c.options) > 0 {
		c.menuInit = true
	}
}

func (c *DropDown) GetSelected() *DropDownOption {
	if len(c.options) == 0 {
		return nil
	}

	return c.options[c.selectedOptionIndex]
}

func (c *DropDown) box(gtx layout.Context, th *theme.Theme, text string, maxWidth unit.Dp) layout.Dimensions {
	borderColor := th.BorderColor
	if c.isOpen || gtx.Source.Focused(&c.clickable) {
		borderColor = th.BorderColorFocused
	}

	border := widget.Border{
		Color:        borderColor,
		Width:        c.borderWidth,
		CornerRadius: c.cornerRadius,
	}

	if maxWidth == 0 {
		maxWidth = unit.Dp(gtx.Constraints.Max.X)
	}

	c.size.X = gtx.Dp(maxWidth)

	return border.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		// calculate the minimum width of the box, considering icon and padding
		cgtx.Constraints.Min.X = cgtx.Dp(maxWidth) - cgtx.Dp(8)
		return layout.Inset{
			Top:    4,
			Bottom: 4,
			Left:   8,
			Right:  4,
		}.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(ccgtx,
				layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(cccgtx, func(c4gtx layout.Context) layout.Dimensions {
						return material.Label(th.Material(), th.TextSize, text).Layout(c4gtx)
					})
				}),
				layout.Rigid(func(cccgtx layout.Context) layout.Dimensions {
					cccgtx.Constraints.Min.X = cccgtx.Dp(16)
					return ExpandIcon.Layout(cccgtx, th.Palette.Fg)
				}),
			)
		})
	})
}

func (c *DropDown) SetSize(size image.Point) {
	c.size = size
}

func (c *DropDown) Changed() bool {
	out := c.changed
	c.changed = false
	return out
}

// Layout the DropDown.
func (c *DropDown) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if c.ShowLabel {
		// Render label above the dropdown
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				label := material.Label(th.Material(), unit.Sp(13), c.Label)
				label.Color = th.TextColor
				return layout.Inset{Bottom: unit.Dp(6)}.Layout(cgtx, label.Layout)
			}),
			layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return c.layoutDropdown(cgtx, th)
			}),
		)
	}
	return c.layoutDropdown(gtx, th)
}

// layoutDropdown renders the actual dropdown without the label
func (c *DropDown) layoutDropdown(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	// Register for input events - this is required for key events to work!
	// Create a clipping area for event.Op to work for keyboard input
	stack := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &c.clickable)
	stack.Pop()

	// Sync isOpen state with ContextArea's active state and keyboard state
	c.isOpen = c.menuContextArea.Active() || c.keyboardOpen

	// Request focus when ContextArea is activated (clicked)
	if c.menuContextArea.Activated() {
		gtx.Execute(key.FocusCmd{Tag: &c.clickable})
	}

	// Handle focus events for TAB navigation
	for {
		ev, ok := gtx.Event(key.FocusFilter{Target: &c.clickable})
		if !ok {
			break
		}
		if fe, ok := ev.(key.FocusEvent); ok {
			if fe.Focus {
				// Dropdown gained focus - could add visual feedback here
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}

	// Handle UP/DOWN arrow keys when focused to cycle through options
	// Also handle Space/Enter to toggle menu, Escape to close
	if gtx.Source.Focused(&c.clickable) {
		for {
			ev, ok := gtx.Event(
				key.Filter{Focus: &c.clickable, Name: key.NameUpArrow},
				key.Filter{Focus: &c.clickable, Name: key.NameDownArrow},
				key.Filter{Focus: &c.clickable, Name: key.NameSpace},
				key.Filter{Focus: &c.clickable, Name: key.NameEnter},
				key.Filter{Focus: &c.clickable, Name: key.NameReturn},
				key.Filter{Focus: &c.clickable, Name: key.NameEscape},
			)
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				switch ke.Name {
				case key.NameUpArrow:
					if c.selectedOptionIndex > 0 {
						c.selectedOptionIndex--
						gtx.Execute(op.InvalidateCmd{})
					}
				case key.NameDownArrow:
					if c.selectedOptionIndex < len(c.options)-1 {
						c.selectedOptionIndex++
						gtx.Execute(op.InvalidateCmd{})
					}
				case key.NameSpace, key.NameEnter, key.NameReturn:
					c.keyboardOpen = !c.keyboardOpen
					if !c.keyboardOpen {
						c.menuContextArea.Dismiss()
					}
					gtx.Execute(op.InvalidateCmd{})
				case key.NameEscape:
					if c.keyboardOpen {
						c.keyboardOpen = false
						c.menuContextArea.Dismiss()
						gtx.Execute(op.InvalidateCmd{})
					}
				}
			}
		}
	}

	// Handle menu item clicks
	for i, opt := range c.options {
		for opt.clickable.Clicked(gtx) {
			c.selectedOptionIndex = i
			c.keyboardOpen = false
			c.menuContextArea.Dismiss()
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	if c.selectedOptionIndex != c.lastSelectedIndex {
		c.changed = true
		if c.onValueChange != nil {
			go c.onValueChange(c.options[c.selectedOptionIndex].Value)
		}
		c.lastSelectedIndex = c.selectedOptionIndex
	}

	// Update menu items only if options change
	if c.menuInit {
		c.menuInit = false
		c.updateMenuItems(th)
	}

	if c.MinWidth == 0 {
		c.MinWidth = unit.Dp(150)
	}

	text := ""
	if c.selectedOptionIndex >= 0 && c.selectedOptionIndex < len(c.options) {
		text = c.options[c.selectedOptionIndex].Text
	}

	box := c.box(gtx, th, text, c.MaxWidth)

	// Layout the clickable area to handle clicks and focus
	clickableDims := c.clickable.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return box
	})

	// Handle click to request focus
	if c.clickable.Clicked(gtx) {
		gtx.Execute(key.FocusCmd{Tag: &c.clickable})
		gtx.Execute(op.InvalidateCmd{})
	}

	// Draw focus ring when focused
	if gtx.Source.Focused(&c.clickable) {
		DrawFocusRing(gtx, th.BorderColorFocused, clickableDims.Size, gtx.Dp(unit.Dp(4)))
	}

	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(cgtx layout.Context) layout.Dimensions {
			return clickableDims
		}),
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			// ContextArea for mouse interaction
			return c.menuContextArea.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				offset := layout.Inset{
					Top:  unit.Dp(float32(box.Size.Y)/ccgtx.Metric.PxPerDp + 1),
					Left: unit.Dp(4),
				}
				return offset.Layout(ccgtx, func(cccgtx layout.Context) layout.Dimensions {
					return c.layoutMenu(cccgtx, th)
				})
			})
		}),
		layout.Expanded(func(cgtx layout.Context) layout.Dimensions {
			// Keyboard-opened menu: render outside ContextArea
			if c.keyboardOpen && !c.menuContextArea.Active() {
				offset := layout.Inset{
					Top:  unit.Dp(float32(box.Size.Y)/cgtx.Metric.PxPerDp + 1),
					Left: unit.Dp(4),
				}
				return offset.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
					return c.layoutMenu(ccgtx, th)
				})
			}
			return layout.Dimensions{}
		}),
	)
}

func (c *DropDown) layoutMenu(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Dp(c.MinWidth)
	if c.MaxWidth != 0 {
		gtx.Constraints.Max.X = gtx.Dp(c.MaxWidth)
	}
	maxMenuHeight := unit.Dp(300)
	if c.MaxMenuHeight != 0 {
		maxMenuHeight = c.MaxMenuHeight
	}
	gtx.Constraints.Max.Y = gtx.Dp(maxMenuHeight)
	m := component.Menu(th.Material(), &c.menu)
	m.SurfaceStyle.Fill = th.DropDownMenuBgColor
	return m.Layout(gtx)
}

// updateMenuItems creates or updates menu items based on options and calculates minWidth.
func (c *DropDown) updateMenuItems(th *theme.Theme) {
	c.menu.Options = c.menu.Options[:0]
	for i, opt := range c.options {
		i, opt := i, opt // capture loop variables
		c.menu.Options = append(c.menu.Options, func(gtx layout.Context) layout.Dimensions {
			if opt.isDivider {
				dv := component.Divider(th.Material())
				dv.Fill = th.BorderColor
				return dv.Layout(gtx)
			}

			itm := component.MenuItem(th.Material(), &opt.clickable, opt.Text)
			if opt.Icon != nil {
				itm.Icon = opt.Icon
				itm.IconColor = opt.IconColor
				itm.IconSize = opt.IconSize
			}

			// Highlight the currently selected option
			if i == c.selectedOptionIndex {
				itm.Label.Color = th.BorderColorFocused
			} else {
				itm.Label.Color = theme.White
			}
			return itm.Layout(gtx)
		})
	}
}
