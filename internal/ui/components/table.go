package components

import (
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type TableColumn struct {
	Title string
	Width unit.Dp
	Flex  float32
}

type TableRow struct {
	Values   []string
	Selected bool
}

type Table struct {
	Columns []string
	Rows    []TableRow

	// ColumnWidths stores calculated or fixed widths for columns.
	ColumnWidths []unit.Dp

	list        widget.List
	horizList   widget.List
	vertScroll  widget.Scrollbar
	horizScroll widget.Scrollbar

	clickables       []*widget.Clickable
	tableClick       widget.Clickable
	SelectedRow      int
	clicked          bool
	doubleClicked    bool
	selectionChanged bool

	// widthsCalculated tracks if we have already measured content once.
	widthsCalculated bool

	// columnDrags handles resizing.
	columnDrags  []gesture.Drag
	columnClicks []gesture.Click
	headerClicks []widget.Clickable

	// Internal measurements for auto-resize
	measuredWidths []unit.Dp

	// Drag state for smooth resizing
	dragIndex      int
	dragStartWidth unit.Dp
	dragStartX     float32

	// OnCopyFeedback is called when a copy operation completes, with the copied text
	OnCopyFeedback func(text string)
}

func (t *Table) Clicked() bool {
	if t.clicked {
		t.clicked = false
		return true
	}
	return false
}

func (t *Table) DoubleClicked() bool {
	if t.doubleClicked {
		t.doubleClicked = false
		return true
	}
	return false
}

func (t *Table) SelectionChanged() bool {
	if t.selectionChanged {
		t.selectionChanged = false
		return true
	}
	return false
}

func (t *Table) FocusTag() interface{} {
	return t
}

func (t *Table) SetData(columns []string, rows []TableRow) {
	t.Columns = columns
	t.Rows = rows
	t.widthsCalculated = false
	t.dragIndex = -1
}

func (t *Table) ResetWidths() {
	t.widthsCalculated = false
	t.ColumnWidths = nil
	t.measuredWidths = nil
	t.dragIndex = -1
}

func (t *Table) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if len(t.Columns) == 0 {
		return layout.Dimensions{}
	}

	// 1. Register for events
	// We need a clipping area for event.Op to work for keyboard input
	stack := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, t)
	stack.Pop()

	if len(t.clickables) < len(t.Rows) {
		for i := len(t.clickables); i < len(t.Rows); i++ {
			t.clickables = append(t.clickables, &widget.Clickable{})
		}
	}

	// 2. Process keyboard events
	// We register multiple filters to ensure all keys are captured in this Gio version
	for {
		e, ok := gtx.Event(
			key.FocusFilter{Target: t},
			key.Filter{Focus: t, Name: key.NameUpArrow},
			key.Filter{Focus: t, Name: key.NameDownArrow},
			key.Filter{Focus: t, Name: key.NameLeftArrow},
			key.Filter{Focus: t, Name: key.NameRightArrow},
			key.Filter{Focus: t, Name: key.NamePageUp},
			key.Filter{Focus: t, Name: key.NamePageDown},
			key.Filter{Focus: t, Name: key.NameHome},
			key.Filter{Focus: t, Name: key.NameEnd},
			key.Filter{Focus: t, Name: key.NameEnter},
			key.Filter{Focus: t, Name: key.NameReturn},
			key.Filter{Focus: t, Name: key.NameSpace},
			key.Filter{Focus: t, Name: key.Name("C"), Optional: key.ModCtrl | key.ModShift},
		)
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press {
			originalRow := t.SelectedRow
			switch ke.Name {
			case key.NameUpArrow:
				if t.SelectedRow > 0 {
					t.SelectedRow--
				}
			case key.NameDownArrow:
				if t.SelectedRow < len(t.Rows)-1 {
					t.SelectedRow++
				}
			case key.NameLeftArrow:
				t.horizList.ScrollBy(-0.1)
			case key.NameRightArrow:
				t.horizList.ScrollBy(0.1)
			case key.NamePageUp:
				t.SelectedRow -= 10
				if t.SelectedRow < 0 {
					t.SelectedRow = 0
				}
			case key.NamePageDown:
				t.SelectedRow += 10
				if t.SelectedRow >= len(t.Rows) {
					t.SelectedRow = len(t.Rows) - 1
				}
			case key.NameHome:
				t.SelectedRow = 0
			case key.NameEnd:
				if len(t.Rows) > 0 {
					t.SelectedRow = len(t.Rows) - 1
				}
			case key.NameEnter, key.NameSpace, key.NameReturn:
				t.clicked = true
				t.doubleClicked = true
			case key.Name("C"):
				if t.SelectedRow >= 0 && t.SelectedRow < len(t.Rows) {
					if ke.Modifiers.Contain(key.ModCtrl | key.ModShift) {
						tsv := t.GetSelectedRowTSV()
						CopyToClipboard(gtx, tsv)
						if t.OnCopyFeedback != nil {
							t.OnCopyFeedback(TruncateText(tsv, 50))
						}
					} else if ke.Modifiers.Contain(key.ModCtrl) {
						name := t.GetSelectedName()
						CopyToClipboard(gtx, name)
						if t.OnCopyFeedback != nil {
							t.OnCopyFeedback(TruncateText(name, 50))
						}
					}
				}
			}

			if t.SelectedRow != originalRow {
				t.selectionChanged = true
				for i := range t.Rows {
					t.Rows[i].Selected = i == t.SelectedRow
				}
			}
			t.scrollToSelected(gtx)
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	// 3. Process mouse events for column resizing
	if len(t.columnDrags) < len(t.Columns) {
		t.columnDrags = make([]gesture.Drag, len(t.Columns))
		t.columnClicks = make([]gesture.Click, len(t.Columns))
	}

	for i := range t.columnDrags {
		for {
			de, ok := t.columnDrags[i].Update(gtx.Metric, gtx.Source, gesture.Horizontal)
			if !ok {
				break
			}
			switch de.Kind {
			case pointer.Press:
				t.dragIndex = i
				t.dragStartX = de.Position.X
				t.dragStartWidth = t.ColumnWidths[i]
			case pointer.Drag:
				if t.dragIndex == i {
					delta := de.Position.X - t.dragStartX
					deltaDp := unit.Dp(delta / float32(gtx.Metric.PxPerDp))
					newWidth := t.dragStartWidth + deltaDp
					if newWidth < unit.Dp(40) {
						newWidth = unit.Dp(40)
					}
					t.ColumnWidths[i] = newWidth
					gtx.Execute(op.InvalidateCmd{})
				}
			case pointer.Release, pointer.Cancel:
				t.dragIndex = -1
			}
		}

		for {
			ce, ok := t.columnClicks[i].Update(gtx.Source)
			if !ok {
				break
			}
			if ce.NumClicks == 2 {
				if len(t.measuredWidths) > i {
					t.ColumnWidths[i] = t.measuredWidths[i]
					gtx.Execute(op.InvalidateCmd{})
				}
			}
		}
	}

	// Calculate widths initially if needed.
	if !t.widthsCalculated {
		t.calculateWidths(gtx, th)
		t.widthsCalculated = true
	}

	// Sync SelectedRow from Rows
	for i, row := range t.Rows {
		if row.Selected {
			t.SelectedRow = i
			break
		}
	}

	t.list.Axis = layout.Vertical

	for t.tableClick.Clicked(gtx) {
		gtx.Execute(key.FocusCmd{Tag: t.FocusTag()})
	}

	headerHeightPx := gtx.Dp(unit.Dp(36))
	rowHeightPx := gtx.Dp(unit.Dp(40))

	return t.tableClick.Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Stack{}.Layout(cgtx,
			layout.Expanded(func(ccgtx layout.Context) layout.Dimensions {
				defer clip.Rect{Max: ccgtx.Constraints.Max}.Push(ccgtx.Ops).Pop()
				event.Op(ccgtx.Ops, t.FocusTag())
				return layout.Dimensions{Size: ccgtx.Constraints.Max}
			}),
			layout.Stacked(func(ccgtx layout.Context) layout.Dimensions {
				t.horizList.Axis = layout.Horizontal
				totalWidth := t.totalWidth(ccgtx)
				viewportWidth := ccgtx.Constraints.Max.X

				// Layer 0: Content area (Horizontally Scrollable)
				return t.horizList.Layout(ccgtx, 1, func(cccgtx layout.Context, _ int) layout.Dimensions {
					w := totalWidth
					if w < viewportWidth {
						w = viewportWidth
					}
					cccgtx.Constraints.Min.X = w
					cccgtx.Constraints.Max.X = w

					return layout.Flex{Axis: layout.Vertical}.Layout(cccgtx,
						layout.Rigid(func(ccccgtx layout.Context) layout.Dimensions {
							return t.layoutHeader(ccccgtx, th)
						}),
						layout.Flexed(1, func(ccccgtx layout.Context) layout.Dimensions {
							return t.list.Layout(ccccgtx, len(t.Rows), func(cccccgtx layout.Context, rowIdx int) layout.Dimensions {
								if rowIdx < 0 || rowIdx >= len(t.Rows) {
									return layout.Dimensions{}
								}
								click := t.clickables[rowIdx]

								for {
									ev, ok := click.Update(cccccgtx)
									if !ok {
										break
									}

									if t.SelectedRow != rowIdx {
										t.selectionChanged = true
									}

									if t.SelectedRow >= 0 && t.SelectedRow < len(t.Rows) {
										t.Rows[t.SelectedRow].Selected = false
									}
									t.SelectedRow = rowIdx
									t.Rows[rowIdx].Selected = true
									t.clicked = true
									if ev.NumClicks == 2 {
										t.doubleClicked = true
									}
									gtx.Execute(key.FocusCmd{Tag: t.FocusTag()})
									cccccgtx.Execute(op.InvalidateCmd{})
								}

								return click.Layout(cccccgtx, func(ccccccgtx layout.Context) layout.Dimensions {
									return t.layoutRow(ccccccgtx, th, t.Rows[rowIdx], rowIdx)
								})
							})
						}),
					)
				})
			}),
			// Fixed Vertical Scrollbar
			layout.Expanded(func(ccgtx layout.Context) layout.Dimensions {
				length := len(t.Rows)
				if length == 0 {
					return layout.Dimensions{}
				}

				viewportHeight := float32(ccgtx.Constraints.Max.Y - headerHeightPx)
				majorAxisSize := float32(length) * float32(rowHeightPx)
				if majorAxisSize <= viewportHeight {
					return layout.Dimensions{}
				}

				totalContentWidth := float32(t.totalWidth(ccgtx))
				viewportWidth := float32(ccgtx.Constraints.Max.X)
				showHoriz := totalContentWidth > viewportWidth

				scrolled := float32(t.list.Position.First)*float32(rowHeightPx) + float32(t.list.Position.Offset)
				start := scrolled / majorAxisSize
				end := (scrolled + viewportHeight) / majorAxisSize

				inset := layout.Inset{Top: unit.Dp(36)}
				if showHoriz {
					inset.Bottom = unit.Dp(10)
				}

				return inset.Layout(ccgtx, func(cccgtx layout.Context) layout.Dimensions {
					return layout.E.Layout(cccgtx, func(ccccgtx layout.Context) layout.Dimensions {
						ccccgtx.Constraints.Min = ccccgtx.Constraints.Max
						dims := material.Scrollbar(th.Material(), &t.vertScroll).Layout(ccccgtx, layout.Vertical, start, end)

						if d := t.vertScroll.ScrollDistance(); d != 0 {
							t.list.ScrollBy(d * float32(length))
							gtx.Execute(op.InvalidateCmd{})
						}
						return dims
					})
				})
			}),
			// Fixed Horizontal Scrollbar
			layout.Expanded(func(ccgtx layout.Context) layout.Dimensions {
				totalContentWidth := float32(t.totalWidth(ccgtx))
				viewportWidth := float32(ccgtx.Constraints.Max.X)
				if totalContentWidth <= viewportWidth {
					return layout.Dimensions{}
				}

				length := len(t.Rows)
				viewportHeight := float32(ccgtx.Constraints.Max.Y - headerHeightPx)
				majorAxisSize := float32(length) * float32(rowHeightPx)
				showVert := majorAxisSize > viewportHeight

				scrolled := float32(t.horizList.Position.First)*totalContentWidth + float32(t.horizList.Position.Offset)
				start := scrolled / totalContentWidth
				end := (scrolled + viewportWidth) / totalContentWidth

				inset := layout.Inset{}
				if showVert {
					inset.Right = unit.Dp(10)
				}

				return inset.Layout(ccgtx, func(cccgtx layout.Context) layout.Dimensions {
					return layout.S.Layout(cccgtx, func(ccccgtx layout.Context) layout.Dimensions {
						ccccgtx.Constraints.Min = ccccgtx.Constraints.Max
						dims := material.Scrollbar(th.Material(), &t.horizScroll).Layout(ccccgtx, layout.Horizontal, start, end)

						if d := t.horizScroll.ScrollDistance(); d != 0 {
							t.horizList.ScrollBy(d * 1.0)
							gtx.Execute(op.InvalidateCmd{})
						}
						return dims
					})
				})
			}),
			layout.Expanded(func(ccgtx layout.Context) layout.Dimensions {
				if gtx.Focused(t.FocusTag()) {
					DrawFocusRing(ccgtx, th.BorderColorFocused, ccgtx.Constraints.Min, gtx.Dp(unit.Dp(4)))
				}
				return layout.Dimensions{Size: ccgtx.Constraints.Min}
			}),
		)
	})
}

func (t *Table) scrollToSelected(gtx layout.Context) {
	if len(t.Rows) == 0 || t.SelectedRow < 0 {
		return
	}

	pos := t.list.Position
	// On first layout, pos.Count will be 0. We fallback to a safe ScrollTo.
	if pos.Count == 0 {
		t.list.ScrollTo(t.SelectedRow)
		return
	}

	// If the row is already visible, don't scroll.
	// pos.First is the index of the first visible item.
	// pos.Count is the number of items partially or fully visible.
	if t.SelectedRow >= pos.First && t.SelectedRow < pos.First+pos.Count-1 {
		return
	}

	// If it's above the visible area, scroll it to the top.
	if t.SelectedRow < pos.First {
		t.list.ScrollTo(t.SelectedRow)
		return
	}

	// If it's below the visible area, scroll so it's at the bottom.
	viewportHeight := float32(gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(36)))
	rowHeight := float32(gtx.Dp(unit.Dp(40)))
	if rowHeight <= 0 {
		return
	}
	rowsInViewport := int(viewportHeight / rowHeight)
	if rowsInViewport > 0 {
		target := t.SelectedRow - rowsInViewport + 1
		if target < 0 {
			target = 0
		}
		// If scrolling down, we ensure we don't jump to top if we can just show the item at bottom
		t.list.ScrollTo(target)
	} else {
		t.list.ScrollTo(t.SelectedRow)
	}
}

func (t *Table) layoutHeader(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	dims := layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		t.layoutColumns(gtx, th, true, -1)...,
	)

	paint.FillShape(gtx.Ops, th.TableBorderColor, clip.Rect{
		Min: image.Pt(0, dims.Size.Y-gtx.Dp(unit.Dp(1))),
		Max: dims.Size,
	}.Op())

	// Draw bottom border for the whole header area if it's not already covered
	// This ensures a clear separation even if columns don't fill the space.
	return dims
}

func (t *Table) layoutRow(gtx layout.Context, th *theme.Theme, row TableRow, index int) layout.Dimensions {
	highlight := row.Selected
	if highlight {
		paint.FillShape(gtx.Ops, th.ActionButtonBgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
	} else if index%2 == 1 {
		paint.FillShape(gtx.Ops, th.TableEvenRowBg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		t.layoutColumns(gtx, th, false, index)...,
	)
}

func (t *Table) layoutColumns(gtx layout.Context, th *theme.Theme, isHeader bool, rowIndex int) []layout.FlexChild {
	var children []layout.FlexChild

	for i, colName := range t.Columns {
		colIdx := i
		headerColName := colName

		var width unit.Dp
		if colIdx < len(t.ColumnWidths) {
			width = t.ColumnWidths[colIdx]
		}

		renderCol := func(cgtx layout.Context) layout.Dimensions {
			// Determine absolute pixel width.
			var colWidth int
			if colIdx < len(t.ColumnWidths) && t.ColumnWidths[colIdx] > 0 {
				colWidth = cgtx.Dp(t.ColumnWidths[colIdx])
			} else {
				colWidth = cgtx.Constraints.Max.X
			}

			// Prepare context for the cell content.
			// We force the width to satisfy the table geometry for all but the last column.
			childGtx := cgtx
			childGtx.Constraints.Min.X = colWidth
			if colIdx < len(t.Columns)-1 {
				childGtx.Constraints.Max.X = colWidth
			}

			var dims layout.Dimensions
			if isHeader {
				// Header: Rigid height to prevent vertical expansion.
				h := cgtx.Dp(unit.Dp(36))
				childGtx.Constraints.Min.Y = h
				childGtx.Constraints.Max.Y = h

				// Use NW alignment. The text fills the column width due to childGtx.
				dims = layout.Stack{Alignment: layout.NW}.Layout(childGtx,
					layout.Stacked(func(gtx2 layout.Context) layout.Dimensions {
						return layout.Inset{
							Top: unit.Dp(10), Bottom: unit.Dp(10),
							Left: unit.Dp(12), Right: unit.Dp(12),
						}.Layout(gtx2, func(gtx3 layout.Context) layout.Dimensions {
							lbl := material.Label(th.Material(), unit.Sp(13), headerColName)
							lbl.Font.Weight = font.Bold
							lbl.Color = th.TextColor
							lbl.MaxLines = 1
							return lbl.Layout(gtx3)
						})
					}),
					layout.Stacked(func(gtx2 layout.Context) layout.Dimensions {
						if colIdx >= len(t.Columns)-1 {
							return layout.Dimensions{}
						}

						// Resize handle: 16dp wide hit-zone.
						hWidth := gtx2.Dp(unit.Dp(16))
						hHalf := hWidth / 2

						// To avoid jitter, we fix the hit-zone position during the drag.
						// The visual column resizes, but the sensitive area stays where the drag started.
						// It will snap to the new boundary on mouse release.
						handleOffset := colWidth
						// Ensure handleOffset is never too small. Use measured width as fallback.
						// A handle offset less than 20px is effectively invisible or positioned incorrectly.
						if handleOffset < gtx2.Dp(unit.Dp(20)) && colIdx < len(t.measuredWidths) {
							handleOffset = gtx2.Dp(t.measuredWidths[colIdx])
						}
						if handleOffset < gtx2.Dp(unit.Dp(20)) {
							handleOffset = gtx2.Dp(unit.Dp(60)) // Minimum fallback width
						}
						if t.dragIndex == colIdx {
							handleOffset = gtx2.Dp(t.dragStartWidth)
						}

						trans := op.Offset(image.Pt(handleOffset-hHalf, 0)).Push(gtx2.Ops)
						rect := image.Rectangle{Max: image.Point{X: hWidth, Y: gtx2.Constraints.Max.Y}}
						stack := clip.Rect(rect).Push(gtx2.Ops)
						pointer.CursorColResize.Add(gtx2.Ops)
						t.columnDrags[colIdx].Add(gtx2.Ops)
						t.columnClicks[colIdx].Add(gtx2.Ops)
						stack.Pop()
						trans.Pop()

						// Return zero size so it doesn't inflate the stack or affect alignment.
						return layout.Dimensions{}
					}),
				)
			} else {
				// Row: Fixed height for smooth scrolling and accurate scrollbar.
				h := cgtx.Dp(unit.Dp(40))
				childGtx.Constraints.Min.Y = h
				childGtx.Constraints.Max.Y = h
				dims = layout.Inset{
					Top: unit.Dp(10), Bottom: unit.Dp(10),
					Left: unit.Dp(12), Right: unit.Dp(12),
				}.Layout(childGtx, func(gtx2 layout.Context) layout.Dimensions {
					var txt string
					if rowIndex >= 0 && rowIndex < len(t.Rows) && colIdx < len(t.Rows[rowIndex].Values) {
						txt = t.Rows[rowIndex].Values[colIdx]
					}
					lbl := material.Label(th.Material(), unit.Sp(13), txt)
					if rowIndex >= 0 && rowIndex < len(t.Rows) && t.Rows[rowIndex].Selected {
						lbl.Color = theme.White
					} else {
						lbl.Color = th.TextColor
					}
					lbl.MaxLines = 1
					return lbl.Layout(gtx2)
				})
			}

			// Ensure the reported width matches the allocated space.
			if colIdx < len(t.Columns)-1 {
				dims.Size.X = colWidth
			} else {
				dims.Size.X = cgtx.Constraints.Max.X
			}

			if colIdx < len(t.Columns)-1 {
				thickness := cgtx.Dp(unit.Dp(1))
				paint.FillShape(cgtx.Ops, th.TableBorderColor, clip.Rect{
					Min: image.Pt(dims.Size.X-thickness, 0),
					Max: image.Pt(dims.Size.X, dims.Size.Y),
				}.Op())
			}
			return dims
		}

		// During drag, treat all columns as rigid to prevent layout jitter
		// from flex-balancing competing with real-time resizing.
		// The last column is always Flexed to ensure it fills the available space
		// and doesn't extend to infinity or leave a gap.
		isLast := colIdx == len(t.Columns)-1
		if (width > 0 || t.dragIndex != -1) && !isLast {
			children = append(children, layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
				return renderCol(cgtx)
			}))
		} else {
			children = append(children, layout.Flexed(1, renderCol))
		}
	}

	return children
}

func (t *Table) calculateWidths(gtx layout.Context, th *theme.Theme) {
	if len(t.Columns) == 0 {
		return
	}

	// Ensure ColumnWidths has the right size
	if len(t.ColumnWidths) != len(t.Columns) {
		newWidths := make([]unit.Dp, len(t.Columns))
		copy(newWidths, t.ColumnWidths)
		t.ColumnWidths = newWidths
	}

	// Ensure measuredWidths has the right size
	if len(t.measuredWidths) != len(t.Columns) {
		t.measuredWidths = make([]unit.Dp, len(t.Columns))
	}

	// Use a temporary ops buffer for measurement to avoid polluting the actual ops
	mops := new(op.Ops)
	mgtx := gtx
	mgtx.Ops = mops
	mgtx.Constraints.Min = image.Point{}
	mgtx.Constraints.Max = image.Point{X: 10000, Y: 10000}

	for i, colName := range t.Columns {
		// Measure header
		lbl := material.Label(th.Material(), unit.Sp(13), colName)
		lbl.Font.Weight = font.Bold
		dims := lbl.Layout(mgtx)
		maxWidth := dims.Size.X

		// Measure first N rows (performance tradeoff)
		sampleSize := 50
		if len(t.Rows) < sampleSize {
			sampleSize = len(t.Rows)
		}

		for r := 0; r < sampleSize; r++ {
			if i < len(t.Rows[r].Values) {
				lbl.Text = t.Rows[r].Values[i]
				lbl.Font.Weight = font.Normal
				dims = lbl.Layout(mgtx)
				if dims.Size.X > maxWidth {
					maxWidth = dims.Size.X
				}
			}
		}

		// Add padding (2 * 12dp)
		idealWidth := unit.Dp(gtx.Metric.PxToDp(maxWidth)) + unit.Dp(24)
		// Minimum width 60dp, Maximum width 400dp
		if idealWidth < unit.Dp(60) {
			idealWidth = unit.Dp(60)
		}
		if idealWidth > unit.Dp(400) {
			idealWidth = unit.Dp(400)
		}

		// Defensive check: ensure measuredWidths is still valid (race condition protection)
		if i < len(t.measuredWidths) {
			t.measuredWidths[i] = idealWidth
		}
		if i < len(t.ColumnWidths) && t.ColumnWidths[i] <= 0 {
			t.ColumnWidths[i] = idealWidth
		}
	}
}

func (t *Table) totalWidth(gtx layout.Context) int {
	w := 0
	for _, cw := range t.ColumnWidths {
		w += gtx.Dp(cw)
	}
	return w
}

func (t *Table) GetSelectedName() string {
	if t.SelectedRow < 0 || t.SelectedRow >= len(t.Rows) {
		return ""
	}

	row := t.Rows[t.SelectedRow]
	if len(row.Values) == 0 {
		return ""
	}

	for i, col := range t.Columns {
		if strings.EqualFold(col, "Name") && i < len(row.Values) {
			return row.Values[i]
		}
	}

	for i, col := range t.Columns {
		if strings.EqualFold(col, "Title") && i < len(row.Values) {
			return row.Values[i]
		}
	}

	return row.Values[0]
}

func (t *Table) GetSelectedRowTSV() string {
	if t.SelectedRow < 0 || t.SelectedRow >= len(t.Rows) {
		return ""
	}

	row := t.Rows[t.SelectedRow]
	return strings.Join(row.Values, "\t")
}
