package models

import "time"

type ErrorInfo struct {
	Message   string
	Severity  string
	Timestamp time.Time
}

type LoadingState struct {
	Loading bool
	Message string
}

type ViewType string

const (
	ViewConnections ViewType = "connections"
	ViewCluster     ViewType = "cluster"
	ViewStreams     ViewType = "streams"
	ViewConsumers   ViewType = "consumers"
	ViewKV          ViewType = "kv"
	ViewObjects     ViewType = "objects"
	ViewServices    ViewType = "services"
	ViewPubSub      ViewType = "pubsub"
	ViewBenchmarks  ViewType = "benchmarks"
	ViewEvents      ViewType = "events"
	ViewBackup      ViewType = "backup"
	ViewSchema      ViewType = "schema"
	ViewAccount     ViewType = "account"
	ViewSettings    ViewType = "settings"
)

type KVItem struct {
	Key      string
	Value    []byte
	Format   string
	Deleted  bool
	Revision uint64
	Created  time.Time
}

type Tab struct {
	ID      string
	Title   string
	View    ViewType
	Context interface{}
}
