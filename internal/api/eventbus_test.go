package api

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventBus_Subscribe_Publish(t *testing.T) {
	eb := NewEventBus(100)
	ch, unsub := eb.Subscribe(nil)
	defer unsub()

	eb.Publish(Event{Type: "device_connected", DeviceID: "dev1", SessionID: "sess1"})

	select {
	case ev := <-ch:
		require.Equal(t, "device_connected", ev.Type)
		require.Equal(t, "dev1", ev.DeviceID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	eb := NewEventBus(100)
	ch1, unsub1 := eb.Subscribe(nil)
	defer unsub1()
	ch2, unsub2 := eb.Subscribe(nil)
	defer unsub2()

	eb.Publish(Event{Type: "device_connected", DeviceID: "dev1"})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); <-ch1 }()
	go func() { defer wg.Done(); <-ch2 }()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	eb := NewEventBus(100)
	ch, unsub := eb.Subscribe(nil)

	eb.Publish(Event{Type: "device_connected", DeviceID: "dev1"})
	ev := <-ch
	require.Equal(t, "device_connected", ev.Type)

	unsub()
	time.Sleep(100 * time.Millisecond)

	_, ok := <-ch
	require.False(t, ok, "channel should be closed after unsubscribe")
}

func TestEventBus_BufferFull_Drops(t *testing.T) {
	eb := NewEventBus(2)
	ch, unsub := eb.Subscribe(nil)
	defer unsub()

	for i := 0; i < 10; i++ {
		eb.Publish(Event{Type: "device_connected", DeviceID: "dev1"})
	}

	time.Sleep(50 * time.Millisecond)

	count := 0
	for {
		select {
		case <-ch:
			count++
		case <-time.After(50 * time.Millisecond):
			goto done
		}
	}
done:
	require.LessOrEqual(t, count, 2)
}

func TestEventBus_Filter(t *testing.T) {
	eb := NewEventBus(100)
	ch, unsub := eb.Subscribe(func(e Event) bool {
		return e.Type == "device_connected"
	})
	defer unsub()

	eb.Publish(Event{Type: "device_disconnected", DeviceID: "dev1"})
	eb.Publish(Event{Type: "device_connected", DeviceID: "dev1"})

	select {
	case ev := <-ch:
		require.Equal(t, "device_connected", ev.Type)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	select {
	case ev := <-ch:
		t.Fatalf("should not receive disconnected event: %v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}
