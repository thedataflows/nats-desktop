package models

import (
	"time"
)

type StreamConfig struct {
	Name      string
	Subjects  []string
	Replicas  int
	Storage   string
	Retention string
	MaxBytes  int64
	MaxAge    time.Duration
}

type StreamMessage struct {
	Stream  string
	Seq     uint64
	Subject string
	Data    []byte
	Headers map[string][]string
	Time    time.Time
}

type MessageViewFormat int

const (
	ViewFormatJSON MessageViewFormat = iota
	ViewFormatText
	ViewFormatHex
)
