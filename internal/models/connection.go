package models

import "time"

type ConnectionContext struct {
	ID        string
	Name      string
	URL       string
	User      string
	Password  string
	Token     string
	NKey      string
	CredsPath string
	CreatedAt time.Time
}

type StreamInfo struct {
	Name      string
	Subjects  []string
	Replicas  int
	Storage   string
	Retention string
	MaxBytes  int64
	MaxAge    time.Duration
	Messages  int64
	Bytes     int64
}

type ConsumerInfo struct {
	StreamName    string
	Name          string
	Durable       bool
	AckPolicy     string
	DeliverPolicy string
	Pending       uint64
	AckPending    uint64
}

type KVStoreInfo struct {
	Name      string
	BucketTTL time.Duration
	MaxBytes  int64
	History   int
	Keys      uint64
	Values    uint64
}

type KVConfig struct {
	Name     string
	History  int
	TTL      time.Duration
	MaxBytes int64
	Replicas int
}

type KVEntry struct {
	Key       string
	Value     []byte
	Revision  uint64
	Created   time.Time
	Operation string
}

type ObjectStoreInfo struct {
	Name     string
	MaxBytes int64
	MaxAge   time.Duration
	Objects  int
	Bytes    int64
}

type ObjectConfig struct {
	Name     string
	TTL      time.Duration
	MaxBytes int64
	Replicas int
}

type ObjectInfo struct {
	Name      string
	Size      uint64
	ModTime   time.Time
	ChunkSize int
	Deleted   bool
	NUID      string
}

type ServiceInfo struct {
	Name      string
	Version   string
	Endpoints []string
	Instances int
}

type ServerInfo struct {
	Name      string
	Host      string
	Port      int
	Version   string
	JetStream bool
	Connected bool
	RTT       time.Duration
}
