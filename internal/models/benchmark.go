package models

import "time"

type BenchmarkResult struct {
	TotalMessages  uint64
	SuccessCount   uint64
	ErrorCount     uint64
	BytesSent      uint64
	BytesReceived  uint64
	Duration       time.Duration
	MessagesPerSec float64
	BytesPerSec    float64
	AvgLatency     time.Duration
	MinLatency     time.Duration
	MaxLatency     time.Duration
	P50Latency     time.Duration
	P95Latency     time.Duration
	P99Latency     time.Duration
}

type BenchmarkConfig struct {
	Type        string
	Subject     string
	Payload     string
	MessageRate int
	Count       int
	Duration    int
	Clients     int
	Size        int
	QueueGroups bool
	JetStream   bool
	OnProgress  func(messages uint64, total uint64)
}
