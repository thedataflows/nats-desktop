package components

import (
	"image"
	"image/color"
	"sync"
	"time"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
	"github.com/thedataflows/nats-desktop/internal/utils"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"
)

type ListStyle struct {
	items      []ListItem
	clickables []*widget.Clickable

	list *widget.List

	th *theme.Theme

	selectedIdx int

	onClick       func(index int)
	onSelect      func(index int)
	onDoubleClick func(index int)
	onTab         func(gtx layout.Context, shift bool)

	dense bool

	focusTag interface{}

	background widget.Clickable

	actions          []ListItemAction
	actionClickables [][]*widget.Clickable
	actionTipAreas   [][]*component.TipArea

	mx *sync.Mutex

	// Double-click detection
	lastClickTime time.Time
	lastClickIdx  int
}

type ListItemAction struct {
	Icon    *widget.Icon
	Tooltip string
	OnClick func(index int)
}

type ListItem struct {
	Title       string
	Subtitle    string
	Description string
	Metadata    string
	Identifier  string
	Icon        *widget.Icon
	Disabled    bool
	Deleted     bool
	Created     time.Time
}

func NewList(th *theme.Theme) *ListStyle {
	return &ListStyle{
		list: &widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
		th:          th,
		selectedIdx: -1,
		mx:          &sync.Mutex{},
	}
}

func (l *ListStyle) SetItems(items []ListItem) {
	l.mx.Lock()
	defer l.mx.Unlock()

	l.items = items
	l.clickables = make([]*widget.Clickable, len(items))
	for i := range l.clickables {
		l.clickables[i] = &widget.Clickable{}
	}

	l.actionClickables = make([][]*widget.Clickable, len(items))
	l.actionTipAreas = make([][]*component.TipArea, len(items))
	for i := range l.actionClickables {
		l.actionClickables[i] = make([]*widget.Clickable, len(l.actions))
		l.actionTipAreas[i] = make([]*component.TipArea, len(l.actions))
		for j := range l.actionClickables[i] {
			l.actionClickables[i][j] = &widget.Clickable{}
			l.actionTipAreas[i][j] = &component.TipArea{}
		}
	}

	l.list.List.Axis = layout.Vertical
	if l.dense {
		l.list.Axis = layout.Vertical
	}
}

func (l *ListStyle) AddAction(icon *widget.Icon, tooltip string, onClick func(index int)) {
	l.mx.Lock()
	defer l.mx.Unlock()

	l.actions = append(l.actions, ListItemAction{
		Icon:    icon,
		Tooltip: tooltip,
		OnClick: onClick,
	})
}

func (l *ListStyle) SetSelected(index int) {
	l.mx.Lock()
	defer l.mx.Unlock()

	l.selectedIdx = index
}

func (l *ListStyle) OnClick(fn func(index int)) {
	l.onClick = fn
}

func (l *ListStyle) OnSelect(fn func(index int)) {
	l.onSelect = fn
}

func (l *ListStyle) OnDoubleClick(fn func(index int)) {
	l.onDoubleClick = fn
}

func (l *ListStyle) SetOnTab(fn func(gtx layout.Context, shift bool)) {
	l.onTab = fn
}

func (l *ListStyle) SetDense(dense bool) {
	l.dense = dense
}

func (l *ListStyle) SetFocusTag(tag interface{}) {
	l.mx.Lock()
	defer l.mx.Unlock()
	if tag == nil {
		l.focusTag = &l.background
		return
	}
	// Always use the list background as the focusable tag to avoid focus loss.
	l.focusTag = &l.background
}

func (l *ListStyle) FocusTag() interface{} {
	l.mx.Lock()
	defer l.mx.Unlock()
	if l.focusTag == nil {
		return &l.background
	}
	return l.focusTag
}

// IsNearEnd returns true if the list is scrolled near the end (within last few items)
func (l *ListStyle) IsNearEnd(threshold int) bool {
	l.mx.Lock()
	defer l.mx.Unlock()
	if len(l.items) == 0 {
		return false
	}
	// Check if we're near the end based on visible items
	lastVisible := l.list.Position.First + l.list.Position.Count
	return lastVisible >= len(l.items)-threshold
}

// ScrollPosition returns the current scroll position
func (l *ListStyle) ScrollPosition() layout.Position {
	l.mx.Lock()
	defer l.mx.Unlock()
	return l.list.Position
}

func (l *ListStyle) SelectNext() {
	l.mx.Lock()
	defer l.mx.Unlock()
	if len(l.items) == 0 {
		return
	}
	l.selectedIdx++
	if l.selectedIdx >= len(l.items) {
		l.selectedIdx = len(l.items) - 1
	}
	l.list.ScrollTo(l.selectedIdx)
	if l.onSelect != nil {
		l.onSelect(l.selectedIdx)
	}
}

func (l *ListStyle) SelectPrev() {
	l.mx.Lock()
	defer l.mx.Unlock()
	if len(l.items) == 0 {
		return
	}
	l.selectedIdx--
	if l.selectedIdx < 0 {
		l.selectedIdx = 0
	}
	l.list.ScrollTo(l.selectedIdx)
	if l.onSelect != nil {
		l.onSelect(l.selectedIdx)
	}
}

func (l *ListStyle) Layout(gtx layout.Context) layout.Dimensions {
	l.mx.Lock()
	items := l.items
	clickables := l.clickables
	selectedIdx := l.selectedIdx
	focusTag := l.focusTag
	actions := l.actions
	actionClickables := l.actionClickables
	l.mx.Unlock()

	if focusTag == nil {
		focusTag = &l.background
	}
	if focusTag != nil {
		for l.background.Clicked(gtx) {
			gtx.Execute(key.FocusCmd{Tag: focusTag})
		}

		// Handle key events
		for {
			e, ok := gtx.Event(
				key.Filter{Focus: focusTag, Name: key.NameUpArrow},
				key.Filter{Focus: focusTag, Name: key.NameDownArrow},
				key.Filter{Focus: focusTag, Name: key.NameEnter},
				key.Filter{Focus: focusTag, Name: key.NameReturn},
			)
			if !ok {
				break
			}
			if x, ok := e.(key.Event); ok && x.State == key.Press {
				switch x.Name {
				case key.NameDownArrow:
					l.SelectNext()
					gtx.Execute(op.InvalidateCmd{})
					l.mx.Lock()
					selectedIdx = l.selectedIdx
					l.mx.Unlock()
				case key.NameUpArrow:
					l.SelectPrev()
					gtx.Execute(op.InvalidateCmd{})
					l.mx.Lock()
					selectedIdx = l.selectedIdx
					l.mx.Unlock()
				case key.NameEnter, key.NameReturn:
					l.mx.Lock()
					idx := l.selectedIdx
					l.mx.Unlock()
					if l.onClick != nil && idx >= 0 && idx < len(items) {
						l.onClick(idx)
					}
					if l.onSelect != nil && idx >= 0 && idx < len(items) {
						l.onSelect(idx)
					}
				}
			}
		}

		// Handle TAB key for focus navigation
		if l.onTab != nil {
			for {
				e, ok := gtx.Event(
					key.Filter{Focus: focusTag, Name: key.NameTab, Optional: key.ModShift},
				)
				if !ok {
					break
				}
				if x, ok := e.(key.Event); ok && x.State == key.Press {
					l.onTab(gtx, x.Modifiers.Contain(key.ModShift))
				}
			}
		}
	}

	if len(items) == 0 {
		material.Label(l.th.Material(), unit.Sp(14), "No items").Layout(gtx)
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	var listDims layout.Dimensions
	listDims = l.background.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		dims := layout.Stack{}.Layout(cgtx,
			layout.Expanded(func(ccgtx layout.Context) layout.Dimensions {
				if focusTag != nil {
					defer clip.Rect{Max: ccgtx.Constraints.Max}.Push(ccgtx.Ops).Pop()
					// Register for pointer events and focus.
					event.Op(ccgtx.Ops, focusTag)
				}
				return layout.Dimensions{Size: ccgtx.Constraints.Max}
			}),
			layout.Stacked(func(ccgtx layout.Context) layout.Dimensions {
				return material.List(l.th.Material(), l.list).Layout(ccgtx, len(items), func(igtx layout.Context, index int) layout.Dimensions {
					if index < 0 || index >= len(clickables) {
						return layout.Dimensions{}
					}

					item := items[index]
					clickable := clickables[index]

					actionClicked := false
					for i, action := range actions {
						for actionClickables[index][i].Clicked(igtx) {
							if action.OnClick != nil {
								action.OnClick(index)
								actionClicked = true
							}
						}
					}

					if !actionClicked && clickable.Clicked(igtx) {
						l.mx.Lock()
						l.selectedIdx = index
						// Double-click detection
						now := time.Now()
						isDoubleClick := index == l.lastClickIdx && now.Sub(l.lastClickTime) < 300*time.Millisecond
						l.lastClickIdx = index
						l.lastClickTime = now
						l.mx.Unlock()

						if focusTag != nil {
							igtx.Execute(key.FocusCmd{Tag: focusTag})
						}
						if l.onSelect != nil {
							l.onSelect(index)
						}
						if isDoubleClick && l.onDoubleClick != nil {
							l.onDoubleClick(index)
						} else if l.onClick != nil {
							l.onClick(index)
						}
					}

					isSelected := index == selectedIdx
					isHovered := clickable.Hovered()
					selectedBg := l.th.ActionButtonBgColor
					selectedText := theme.White

					return layout.UniformInset(unit.Dp(8)).Layout(igtx, func(lgtx1 layout.Context) layout.Dimensions {
						return clickable.Layout(lgtx1, func(lgtx2 layout.Context) layout.Dimensions {
							bg := color.NRGBA{}
							if isSelected {
								bg = selectedBg
							} else if isHovered {
								bg = WithAlpha(l.th.BorderColor, 8)
							}

							contentMacro := op.Record(lgtx2.Ops)
							contentDims := layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(lgtx2, func(lgtx3 layout.Context) layout.Dimensions {
								// Ensure a stable minimum height for the item content.
								lgtx3.Constraints.Min.Y = lgtx3.Dp(unit.Dp(36))
								return layout.Stack{Alignment: layout.E}.Layout(lgtx3,
									layout.Stacked(func(lgtx4 layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(lgtx4,
											layout.Rigid(func(lgtx5 layout.Context) layout.Dimensions {
												if item.Icon == nil {
													return layout.Dimensions{}
												}
												return layout.Inset{Right: unit.Dp(12)}.Layout(lgtx5, func(lgtx6 layout.Context) layout.Dimensions {
													iconColor := l.th.TextColor
													if isSelected {
														iconColor = selectedText
													} else {
														iconColor = WithAlpha(iconColor, 160)
													}
													if item.Deleted {
														iconColor = WithAlpha(iconColor, 120)
													}
													lgtx6.Constraints.Min = image.Pt(lgtx6.Dp(unit.Dp(24)), lgtx6.Dp(unit.Dp(24)))
													lgtx6.Constraints.Max = lgtx6.Constraints.Min
													return item.Icon.Layout(lgtx6, iconColor)
												})
											}),
											layout.Flexed(1, func(lgtx5 layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Vertical}.Layout(lgtx5,
													layout.Rigid(func(lgtx6 layout.Context) layout.Dimensions {
														titleColor := l.th.TextColor
														if isSelected {
															titleColor = selectedText
														} else {
															titleColor = WithAlpha(titleColor, 230)
														}
														text := item.Title
														if item.Deleted {
															text = utils.Strikethrough(text)
														}
														title := material.Label(l.th.Material(), unit.Sp(14), text)
														title.Font.Weight = font.Medium
														title.Color = titleColor

														if item.Deleted {
															title.Color = WithAlpha(titleColor, 150)
														}
														return title.Layout(lgtx6)
													}),
													layout.Rigid(func(lgtx6 layout.Context) layout.Dimensions {
														if item.Subtitle == "" {
															return layout.Dimensions{}
														}
														subtitleColor := WithAlpha(l.th.TextColor, 140)
														if isSelected {
															subtitleColor = WithAlpha(selectedText, 200)
														}
														subtitle := material.Label(l.th.Material(), unit.Sp(12), item.Subtitle)
														subtitle.Color = subtitleColor

														if item.Deleted {
															subtitle.Color = WithAlpha(subtitleColor, 150)
														}

														return subtitle.Layout(lgtx6)
													}),
													layout.Rigid(func(lgtx6 layout.Context) layout.Dimensions {
														if item.Metadata == "" {
															return layout.Dimensions{}
														}
														metadataColor := WithAlpha(l.th.TextColor, 100)
														if isSelected {
															metadataColor = WithAlpha(selectedText, 160)
														}
														metadata := material.Label(l.th.Material(), unit.Sp(11), item.Metadata)
														metadata.Color = metadataColor
														return metadata.Layout(lgtx6)
													}),
												)
											}),
										)
									}),
									layout.Expanded(func(lgtx4 layout.Context) layout.Dimensions {
										if !isHovered && !isSelected {
											return layout.Dimensions{}
										}
										return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(lgtx4,
											layout.Rigid(func(lgtx5 layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal}.Layout(lgtx5,
													l.renderActions(lgtx5, index, isSelected)...,
												)
											}),
										)
									}),
								)
							})
							contentCall := contentMacro.Stop()

							if bg.A != 0 {
								defer clip.Rect(image.Rectangle{Max: contentDims.Size}).Push(lgtx2.Ops).Pop()
								paint.Fill(lgtx2.Ops, bg)
							}
							contentCall.Add(lgtx2.Ops)
							return contentDims
						})
					})
				})
			}),
		)
		return dims
	})

	if focusTag != nil && gtx.Focused(focusTag) {
		DrawFocusRing(gtx, l.th.BorderColorFocused, listDims.Size, gtx.Dp(unit.Dp(4)))
	}

	return listDims
}

func (l *ListStyle) renderActions(gtx layout.Context, index int, isSelected bool) []layout.FlexChild {
	l.mx.Lock()
	actions := l.actions
	clickables := l.actionClickables[index]
	tipAreas := l.actionTipAreas[index]
	l.mx.Unlock()

	var children []layout.FlexChild
	for i := range actions {
		currAction := actions[i]
		currClickable := clickables[i]
		currTipArea := tipAreas[i]

		children = append(children, layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
				ib := IconButton{
					Icon:      currAction.Icon,
					Size:      unit.Dp(24),
					Clickable: currClickable,
					Tooltip:   currAction.Tooltip,
					TipArea:   currTipArea,
				}
				if isSelected {
					ib.Color = theme.White
				} else {
					ib.Color = l.th.TextColor
				}
				ib.BackgroundColor = color.NRGBA{A: 0}
				ib.BackgroundColorHover = color.NRGBA{A: 0}
				return ib.Layout(ccgtx, l.th)
			})
		}))
	}
	return children
}

type GridStyle struct {
	items      []GridItem
	clickables []*widget.Clickable

	list *widget.List

	th *theme.Theme

	columns int

	onClick func(index int)

	dense bool

	mx *sync.Mutex
}

type GridItem struct {
	Title    string
	Subtitle string
	Value    string
	Metadata string
}

func NewGrid(th *theme.Theme, columns int) *GridStyle {
	return &GridStyle{
		list: &widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
		th:      th,
		columns: columns,
		mx:      &sync.Mutex{},
	}
}

func (g *GridStyle) SetItems(items []GridItem) {
	g.mx.Lock()
	defer g.mx.Unlock()

	g.items = items
	g.clickables = make([]*widget.Clickable, len(items))
	for i := range g.clickables {
		g.clickables[i] = &widget.Clickable{}
	}
}

func (g *GridStyle) OnClick(fn func(index int)) {
	g.onClick = fn
}

func (g *GridStyle) SetDense(dense bool) {
	g.dense = dense
}

func (g *GridStyle) Layout(gtx layout.Context) layout.Dimensions {
	g.mx.Lock()
	items := g.items
	clickables := g.clickables
	g.mx.Unlock()

	if len(items) == 0 {
		material.Label(g.th.Material(), unit.Sp(14), "No items").Layout(gtx)
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	return material.List(g.th.Material(), g.list).Layout(gtx, len(items), func(cgtx layout.Context, index int) layout.Dimensions {
		if index < 0 || index >= len(clickables) {
			return layout.Dimensions{}
		}

		item := items[index]
		clickable := clickables[index]

		if clickable.Clicked(cgtx) {
			if g.onClick != nil {
				g.onClick(index)
			}
		}

		isHovered := clickable.Hovered() || cgtx.Focused(clickable)

		return layout.UniformInset(unit.Dp(4)).Layout(cgtx, func(ccgtx layout.Context) layout.Dimensions {
			return clickable.Layout(ccgtx, func(cccgtx layout.Context) layout.Dimensions {
				bg := color.NRGBA{}
				if isHovered {
					bg = WithAlpha(g.th.BorderColor, 8)
				}

				return layout.Stack{}.Layout(cccgtx,
					layout.Stacked(func(ccccgtx layout.Context) layout.Dimensions {
						if bg.A == 0 {
							return layout.Dimensions{}
						}
						defer clip.Rect(image.Rectangle{Max: ccccgtx.Constraints.Min}).Push(ccccgtx.Ops).Pop()
						paint.Fill(ccccgtx.Ops, bg)
						return layout.Dimensions{Size: ccccgtx.Constraints.Min}
					}),
					layout.Stacked(func(ccccgtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(8)).Layout(ccccgtx, func(cccccgtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(cccccgtx,
								layout.Rigid(func(ccccccgtx layout.Context) layout.Dimensions {
									if g.columns > 0 {
										ccccccgtx.Constraints.Min.X = ccccccgtx.Dp(unit.Dp(200 * g.columns / 3))
									}
									if item.Title != "" {
										title := material.Label(g.th.Material(), unit.Sp(14), item.Title)
										title.Color = WithAlpha(g.th.TextColor, 230)
										return title.Layout(ccccccgtx)
									}
									return layout.Dimensions{}
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Rigid(func(ccccccgtx layout.Context) layout.Dimensions {
									if item.Subtitle != "" {
										subtitle := material.Label(g.th.Material(), unit.Sp(14), item.Subtitle)
										subtitle.Color = WithAlpha(g.th.TextColor, 180)
										return subtitle.Layout(ccccccgtx)
									}
									return layout.Dimensions{}
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Rigid(func(ccccccgtx layout.Context) layout.Dimensions {
									if item.Value != "" {
										value := material.Label(g.th.Material(), unit.Sp(14), item.Value)
										value.Color = WithAlpha(g.th.TextColor, 140)
										return value.Layout(ccccccgtx)
									}
									return layout.Dimensions{}
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Rigid(func(ccccccgtx layout.Context) layout.Dimensions {
									if item.Metadata != "" {
										metadata := material.Label(g.th.Material(), unit.Sp(12), item.Metadata)
										metadata.Color = WithAlpha(g.th.TextColor, 100)
										return metadata.Layout(ccccccgtx)
									}
									return layout.Dimensions{}
								}),
							)
						})
					}),
				)
			})
		})
	})
}
