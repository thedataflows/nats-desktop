package views

import (
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"
	log "github.com/thedataflows/go-lib-log"

	"github.com/thedataflows/nats-desktop/internal/icons"
	"github.com/thedataflows/nats-desktop/internal/ui/components"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

// BaseView provides common functionality for all resource views
type BaseView struct {
	App App

	// Common buttons
	AddBtn     widget.Clickable
	RefreshBtn widget.Clickable
	DeleteBtn  widget.Clickable

	// State
	SelectedIdx int
	EmptyState  bool
	Loading     bool

	// Components
	Table        components.Table
	Paginator    *components.Pagination
	Split        components.SplitView
	ConfirmModal *components.Prompt

	// Search
	SearchEditor   *components.InputField
	LastFilterTime time.Time
	FilterQuery    string

	// Pagination
	PerPage int

	// Focus restoration
	RestoreListFocus bool

	// Navigation
	Next, Prev any
}

// NewBaseView creates a new BaseView with common initialization
func NewBaseView(columns []string, perPage int) *BaseView {
	return &BaseView{
		SelectedIdx:  -1,
		EmptyState:   true,
		PerPage:      perPage,
		Paginator:    components.NewPagination(1),
		Table:        components.Table{Columns: columns},
		SearchEditor: components.NewInputField(""),
		Split: components.SplitView{
			Resize: component.Resize{
				Ratio: 0.7,
			},
			BarWidth: unit.Dp(2),
		},
		ConfirmModal: components.NewPrompt("Confirm Action", "Are you sure?", components.ModalTypeWarn, components.Option{Text: "Confirm"}, components.Option{Text: "Cancel"}),
	}
}

// LayoutBase performs the common layout structure for views
func (b *BaseView) LayoutBase(gtx layout.Context, th *theme.Theme, title string,
	headerExtra, actions, content layout.Widget) layout.Dimensions {

	// Handle deferred filter
	if !b.LastFilterTime.IsZero() {
		if gtx.Now.Sub(b.LastFilterTime) > 300*time.Millisecond {
			b.LastFilterTime = time.Time{}
		} else {
			gtx.Execute(op.InvalidateCmd{At: b.LastFilterTime.Add(300 * time.Millisecond)})
		}
	}

	// Background
	paint.FillShape(gtx.Ops, th.Palette.Bg, clip.Rect{
		Max: gtx.Constraints.Max,
	}.Op())

	// Main layout
	return layout.UniformInset(unit.Dp(32)).Layout(gtx, func(cgtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(cgtx,
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				return b.layoutHeader(ccgtx, th, title, headerExtra)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
				if actions != nil {
					return actions(ccgtx)
				}
				return b.layoutDefaultActions(ccgtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
				if content != nil {
					return content(ccgtx)
				}
				return layout.Dimensions{}
			}),
		)
	})
}

// layoutHeader renders the common header with title and optional extra content
func (b *BaseView) layoutHeader(gtx layout.Context, th *theme.Theme, title string, extra layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(cgtx,
				layout.Rigid(func(ccgtx layout.Context) layout.Dimensions {
					header := material.Label(th.Material(), unit.Sp(24), title)
					header.Color = th.TextColor
					return header.Layout(ccgtx)
				}),
				layout.Flexed(1, func(ccgtx layout.Context) layout.Dimensions {
					if extra != nil {
						return extra(ccgtx)
					}
					return layout.Dimensions{}
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			borderColor := th.TableBorderColor
			paint.FillShape(cgtx.Ops, borderColor, clip.Rect{
				Max: image.Pt(cgtx.Constraints.Max.X, cgtx.Dp(unit.Dp(1))),
			}.Op())
			return layout.Dimensions{}
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
	)
}

// layoutDefaultActions renders the default action buttons (Add, Refresh, Delete, Search)
func (b *BaseView) layoutDefaultActions(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	isSelected := b.SelectedIdx >= 0

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			btn := components.Button(th, &b.AddBtn, icons.ContentAddCircle, components.IconPositionStart, "Create")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			btn := components.SecondaryButton(th, &b.RefreshBtn, icons.NavigationRefresh, components.IconPositionStart, "Refresh")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(cgtx layout.Context) layout.Dimensions {
			if !isSelected {
				cgtx = cgtx.Disabled()
			}
			btn := components.SecondaryButton(th, &b.DeleteBtn, icons.ActionDelete, components.IconPositionStart, "Delete")
			return btn.Layout(cgtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(cgtx layout.Context) layout.Dimensions {
			return b.SearchEditor.Layout(cgtx, th)
		}),
	)
}

// HandleDeleteConfirmation shows a delete confirmation modal
func (b *BaseView) HandleDeleteConfirmation(itemName string, onConfirm func()) {
	b.ConfirmModal = components.NewPrompt("Confirm Delete", "Are you sure you want to delete '"+itemName+"'?", components.ModalTypeWarn, components.Option{Text: "Confirm"}, components.Option{Text: "Cancel"})
	b.ConfirmModal.SetOnSubmit(func(selectedOption string, remember bool) {
		if selectedOption == "Confirm" {
			log.Logger().Info().Str("action", "delete").Str("item", itemName).Msg("User confirmed delete")
			if onConfirm != nil {
				onConfirm()
			}
		} else {
			log.Logger().Debug().Str("action", "delete").Str("item", itemName).Msg("User cancelled delete")
		}
		b.SelectedIdx = -1
	})
	b.ConfirmModal.Show()
}

// RefreshAsync performs an async refresh operation with proper loading state
func (b *BaseView) RefreshAsync(fetchFunc func() error, onComplete func()) {
	if b.Loading {
		return
	}

	b.Loading = true
	go func() {
		defer func() {
			b.Loading = false
			if onComplete != nil {
				onComplete()
			}
		}()

		if err := fetchFunc(); err != nil {
			// Error handling can be customized by the view
		}
	}()
}

// FilterItems filters a slice of items based on a query string
func FilterItems[T any](items []T, query string, matchFunc func(T) bool) []T {
	if query == "" {
		return items
	}

	filtered := make([]T, 0)
	for _, item := range items {
		if matchFunc(item) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// BuildTableRows creates table rows from a slice of items with pagination
func BuildTableRows[T any](items []T, currentPage, perPage int, toRow func(T, int) components.TableRow, selectedIdx int) []components.TableRow {
	startIdx := (currentPage - 1) * perPage
	endIdx := startIdx + perPage
	if endIdx > len(items) {
		endIdx = len(items)
	}
	if startIdx < 0 || startIdx >= len(items) {
		return []components.TableRow{}
	}

	pageItems := items[startIdx:endIdx]
	rows := make([]components.TableRow, len(pageItems))
	for i, item := range pageItems {
		rows[i] = toRow(item, startIdx+i)
		rows[i].Selected = (startIdx + i) == selectedIdx
	}
	return rows
}

// GetPageSlice returns a slice of items for the current page
func GetPageSlice[T any](items []T, currentPage, perPage int) []T {
	startIdx := (currentPage - 1) * perPage
	endIdx := startIdx + perPage
	if endIdx > len(items) {
		endIdx = len(items)
	}
	if startIdx < 0 || startIdx >= len(items) {
		return []T{}
	}
	return items[startIdx:endIdx]
}
