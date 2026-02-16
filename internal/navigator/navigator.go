package navigator

import (
	"image"

	"github.com/thedataflows/nats-desktop/internal/ui/theme"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/thedataflows/nats-desktop/internal/sidebar"
)

type Navigator struct {
	views   map[any]View
	current any

	*sidebar.Sidebar

	isConnectedFunc func() bool
}

func New() *Navigator {
	return &Navigator{
		views:   make(map[any]View),
		Sidebar: sidebar.New(),
	}
}

func (n *Navigator) SetConnectionCheckFunc(f func() bool) {
	n.isConnectedFunc = f
}

func (n *Navigator) UpdateDisabledStates() {
	if n.isConnectedFunc == nil {
		return
	}

	isConnected := n.isConnectedFunc()

	// List of views that require a connection (all except Connections and Preferences)
	viewsRequiringConnection := []PageId{
		ClusterPageId,
		StreamsPageId,
		ConsumersPageId,
		KVPageId,
		ObjectsPageId,
		ServicesPageId,
		PubSubPageId,
		BenchmarksPageId,
		EventsPageId,
		BackupPageId,
		SchemaPageId,
		AccountPageId,
		TracePageId,
		CounterPageId,
		AuditPageId,
	}

	for _, pageId := range viewsRequiringConnection {
		n.Sidebar.SetItemDisabled(pageId, !isConnected)
	}
}

func (n *Navigator) IsViewDisabled(id PageId) bool {
	return n.Sidebar.IsItemDisabled(id)
}

func (n *Navigator) Register(view View) {
	detail := view.Info()

	n.views[detail.ID] = view
	if n.current == nil {
		n.current = detail.ID
		view.OnEnter()
	}

	n.Sidebar.AddNavItem(sidebar.Item{
		Tag:  detail.ID,
		Name: detail.Title,
		Icon: detail.Icon,
	})
}

func (n *Navigator) SwitchTo(id any) {
	// Check if the view is disabled
	if pageId, ok := id.(PageId); ok {
		if n.IsViewDisabled(pageId) {
			return
		}
	}

	view, ok := n.views[id]
	if !ok {
		return
	}

	if n.current != nil && n.current != id {
		if oldView, ok := n.views[n.current]; ok {
			if l, ok := oldView.(Leavable); ok {
				l.OnLeave()
			}
		}
	}

	n.current = id
	view.OnEnter()
}

func (n *Navigator) Current() View {
	return n.views[n.current]
}

func (n *Navigator) Update() {
	n.UpdateDisabledStates()

	if n.Sidebar.Changed() {
		current := n.Sidebar.Current()
		n.SwitchTo(current)
	}
}

func (n *Navigator) FirstFocusTag() any {
	return n.Sidebar.FirstFocusTag()
}

func (n *Navigator) LastFocusTag() any {
	return n.Sidebar.LastFocusTag()
}

func (n *Navigator) SetNavigationLinks(next, prev any) {
	n.Sidebar.SetNavigationLinks(next, prev)
}

func (n *Navigator) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	gtx.Constraints.Max.X = gtx.Dp(unit.Dp(110))
	gtx.Constraints.Min = gtx.Constraints.Max

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx1 layout.Context) layout.Dimensions {
			bgColor := th.SideBarBgColor
			paint.FillShape(gtx1.Ops, bgColor, clip.Rect{
				Max: gtx1.Constraints.Max,
			}.Op())

			borderColor := th.TableBorderColor
			borderWidth := gtx1.Dp(unit.Dp(1))
			paint.FillShape(gtx1.Ops, borderColor, clip.Rect{
				Max: image.Pt(borderWidth, gtx1.Constraints.Max.Y),
			}.Op())
			return layout.Dimensions{}
		}),
		layout.Stacked(func(gtx1 layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx1, func(gtx2 layout.Context) layout.Dimensions {
				return n.Sidebar.Layout(gtx2, th)
			})
		}),
	)
}
