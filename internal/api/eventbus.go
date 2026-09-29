package api

import (
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/event"
)

type Event = event.Event

var (
	NewEventBus = event.NewEventBus
)

type EventBusInterface = event.EventBusInterface
