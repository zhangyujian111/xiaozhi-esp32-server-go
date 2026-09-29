package ws

import (
	"sync"
	"testing"
	"time"
)

func TestSessionManager_Register_Get_Remove(t *testing.T) {
	sm := NewSessionManager()

	s1 := NewChatSession("device-1", "session-1")
	if err := sm.Register(s1); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	s2 := NewChatSession("device-2", "session-2")
	if err := sm.Register(s2); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	if sm.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", sm.Count())
	}

	retrieved, ok := sm.Get("session-1")
	if !ok {
		t.Fatal("Get(session-1) returned not found")
	}
	if retrieved.ID() != "session-1" {
		t.Errorf("retrieved.ID() = %q, want session-1", retrieved.ID())
	}

	retrievedByDev, ok := sm.GetByDevice("device-1")
	if !ok {
		t.Fatal("GetByDevice(device-1) returned not found")
	}
	if retrievedByDev.ID() != "session-1" {
		t.Errorf("retrievedByDev.ID() = %q, want session-1", retrievedByDev.ID())
	}

	sm.Remove("session-1")
	if sm.Count() != 1 {
		t.Fatalf("after Remove Count() = %d, want 1", sm.Count())
	}

	_, ok = sm.Get("session-1")
	if ok {
		t.Error("after Remove Get(session-1) should return not found")
	}
}

func TestSessionManager_ByDevice(t *testing.T) {
	sm := NewSessionManager()

	s := NewChatSession("device-abc", "session-xyz")
	sm.Register(s)

	retrieved, ok := sm.GetByDevice("device-abc")
	if !ok {
		t.Fatal("GetByDevice(device-abc) returned not found")
	}
	if retrieved.DeviceID() != "device-abc" {
		t.Errorf("DeviceID() = %q, want device-abc", retrieved.DeviceID())
	}

	_, ok = sm.GetByDevice("nonexistent")
	if ok {
		t.Error("GetByDevice(nonexistent) should return not found")
	}
}

func TestSessionManager_DuplicateRegister_Fails(t *testing.T) {
	sm := NewSessionManager()

	s1 := NewChatSession("device-1", "session-1")
	if err := sm.Register(s1); err != nil {
		t.Fatalf("first Register() failed: %v", err)
	}

	s2 := NewChatSession("device-1", "session-2")
	if err := sm.Register(s2); err == nil {
		t.Fatal("duplicate device Register() should return error, got nil")
	}

	if sm.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (duplicate rejected)", sm.Count())
	}
}

func TestSessionManager_Concurrent(t *testing.T) {
	sm := NewSessionManager()
	const n = 1000

	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			deviceID := "device-concurrent"
			sessionID := sessionIDFromInt(i)
			s := NewChatSession(deviceID, sessionID)
			sm.Register(s)
		}()
	}

	wg.Wait()

	count := sm.Count()
	if count == 0 {
		t.Fatal("Count() = 0 after concurrent registrations, expected at least 1")
	}
}

func sessionIDFromInt(i int) string {
	return time.Now().Format(time.RFC3339Nano) + "-" + string(rune('a'+i%26))
}
