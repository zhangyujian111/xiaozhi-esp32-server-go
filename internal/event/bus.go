package event

import (
	"sync"
	"time"
)

type Event struct {
	Type      string    `json:"type"`
	DeviceID  string    `json:"device_id"`
	SessionID string    `json:"session_id,omitempty"`
	Timestamp time.Time `json:"ts"`
}

type EventBus struct {
	subscribers sync.Map
	bufferSize  int
}

type subscription struct {
	ch     chan Event
	done   chan struct{}
	filter func(Event) bool
}

func NewEventBus(bufferSize int) *EventBus {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &EventBus{bufferSize: bufferSize}
}

func (eb *EventBus) Subscribe(filter func(Event) bool) (<-chan Event, func()) {
	sub := &subscription{
		ch:     make(chan Event, eb.bufferSize),
		done:   make(chan struct{}),
		filter: filter,
	}
	eb.subscribers.Store(sub, sub)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-sub.done
		eb.subscribers.Delete(sub)
		close(sub.ch)
	}()

	unsubscribe := func() {
		close(sub.done)
	}
	return sub.ch, unsubscribe
}

func (eb *EventBus) Publish(event Event) {
	event.Timestamp = time.Now()
	eb.subscribers.Range(func(key, value interface{}) bool {
		sub := value.(*subscription)
		if sub.filter == nil || sub.filter(event) {
			select {
			case sub.ch <- event:
			default:
			}
		}
		return true
	})
}

var _ EventBusInterface = (*EventBus)(nil)

type EventBusInterface interface {
	Subscribe(filter func(Event) bool) (<-chan Event, func())
	Publish(event Event)
}
