package navigator

import (
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/thedataflows/nats-desktop/internal/shortcuts"
	"github.com/thedataflows/nats-desktop/internal/ui/theme"
)

type PageId string

const (
	ConnectionsPageId PageId = "connections"
	ClusterPageId     PageId = "cluster"
	StreamsPageId     PageId = "streams"
	ConsumersPageId   PageId = "consumers"
	KVPageId          PageId = "kv"
	ObjectsPageId     PageId = "objects"
	ServicesPageId    PageId = "services"
	PubSubPageId      PageId = "pubsub"
	BenchmarksPageId  PageId = "benchmarks"
	EventsPageId      PageId = "events"
	BackupPageId      PageId = "backup"
	SchemaPageId      PageId = "schema"
	AccountPageId     PageId = "account"
	TracePageId       PageId = "trace"
	CounterPageId     PageId = "counter"
	PreferencesPageId PageId = "preferences"
	AuditPageId       PageId = "audit"
)

type Info struct {
	ID    PageId
	Title string
	Icon  *widget.Icon
}

type View interface {
	Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions
	Info() Info
	OnEnter()
	FirstFocusTag() any
	LastFocusTag() any
	SetNavigation(next, prev any)
	HandleShortcuts(gtx layout.Context) bool
	GetShortcutsHelp() []shortcuts.Shortcut
}

type Leavable interface {
	OnLeave()
}

type Refreshable interface {
	Refresh()
}
