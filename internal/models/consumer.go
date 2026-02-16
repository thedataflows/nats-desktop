package models

type ConsumerConfig struct {
	Name          string
	Durable       string
	AckPolicy     string
	DeliverPolicy string
	FilterSubject string
	MaxDeliver    int
	MaxAckPending int
	ReplayPolicy  string
}
