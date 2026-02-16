package events

import "github.com/thedataflows/nats-desktop/internal/models"

type Event interface{}

type NavigateEvent struct {
	Target  models.ViewType
	Context interface{}
}

type ConnectionChangedEvent struct {
	OldContext *models.ConnectionContext
	NewContext *models.ConnectionContext
}

type StreamCreatedEvent struct {
	Stream *models.StreamInfo
}

type StreamUpdatedEvent struct {
	Stream *models.StreamInfo
}

type StreamDeletedEvent struct {
	StreamName string
}

type NotificationEvent struct {
	Title   string
	Message string
	Level   string
}
