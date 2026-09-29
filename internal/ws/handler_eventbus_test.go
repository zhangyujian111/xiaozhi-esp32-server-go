package ws

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/event"
)

type fakeEventBus struct {
	mu       sync.Mutex
	events   []event.Event
	publishC chan event.Event
}

func (f *fakeEventBus) Subscribe(filter func(event.Event) bool) (<-chan event.Event, func()) {
	ch := make(chan event.Event, 100)
	done := make(chan struct{})
	if f.publishC != nil {
		go func() {
			for {
				select {
				case e := <-f.publishC:
					f.mu.Lock()
					f.events = append(f.events, e)
					f.mu.Unlock()
					select {
					case ch <- e:
					case <-done:
						return
					}
				case <-done:
					return
				}
			}
		}()
	}
	return ch, func() { close(done) }
}

func (f *fakeEventBus) Publish(e event.Event) {
	f.mu.Lock()
	f.events = append(f.events, e)
	f.mu.Unlock()
	if f.publishC != nil {
		select {
		case f.publishC <- e:
		default:
		}
	}
}

func TestServeConn_PublishesDeviceConnectedEvent(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	bus := &fakeEventBus{publishC: make(chan event.Event, 10)}
	h := NewHandler(sm, &mockAuth{err: nil}, logger)
	h.SetEventBus(bus)

	server := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer server.Close()

	wsURL := "ws://" + server.URL[7:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Device-Id":     {"test-device-publish"},
		"Authorization": {"Bearer valid-token"},
	})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	msgCh := make(chan *interface{}, 1)
	go func() {
		var resp interface{}
		if err := conn.ReadJSON(&resp); err == nil {
			msgCh <- &resp
		}
	}()

	hello := map[string]interface{}{"type": "hello", "version": 1, "transport": "websocket"}
	if err := conn.WriteJSON(hello); err != nil {
		t.Fatalf("write hello failed: %v", err)
	}

	select {
	case <-msgCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for hello response")
	}

	time.Sleep(100 * time.Millisecond)

	found := false
	for _, e := range bus.events {
		if e.Type == "device_connected" && e.DeviceID == "test-device-publish" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected device_connected event for test-device-publish, got events: %v", bus.events)
	}
}

func TestServeConn_PublishesDeviceDisconnectedEvent(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	bus := &fakeEventBus{publishC: make(chan event.Event, 10)}
	h := NewHandler(sm, &mockAuth{err: nil}, logger)
	h.SetEventBus(bus)

	server := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer server.Close()

	wsURL := "ws://" + server.URL[7:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Device-Id":     {"test-device-disconnect"},
		"Authorization": {"Bearer valid-token"},
	})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	msgCh := make(chan *interface{}, 1)
	go func() {
		var resp interface{}
		if err := conn.ReadJSON(&resp); err == nil {
			msgCh <- &resp
		}
	}()

	hello := map[string]interface{}{"type": "hello", "version": 1, "transport": "websocket"}
	if err := conn.WriteJSON(hello); err != nil {
		t.Fatalf("write hello failed: %v", err)
	}

	select {
	case <-msgCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for hello response")
	}

	initialCount := len(bus.events)

	conn.Close()
	time.Sleep(100 * time.Millisecond)

	found := false
	for _, e := range bus.events[initialCount:] {
		if e.Type == "device_disconnected" && e.DeviceID == "test-device-disconnect" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected device_disconnected event for test-device-disconnect, got events: %v", bus.events)
	}
}
