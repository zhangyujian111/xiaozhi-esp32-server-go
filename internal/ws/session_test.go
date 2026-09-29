package ws

import (
	"testing"
	"time"
)

func TestChatSession_StateTransitions(t *testing.T) {
	s := NewChatSession("test-device", "test-session")

	if s.State() != StateIdle {
		t.Fatalf("expected initial state=idle, got %s", s.State())
	}

	if err := s.TransitionTo(StateListening); err != nil {
		t.Fatalf("transition idle→listening failed: %v", err)
	}
	if s.State() != StateListening {
		t.Fatalf("expected state=listening after transition, got %s", s.State())
	}

	if err := s.TransitionTo(StateThinking); err != nil {
		t.Fatalf("transition listening→thinking failed: %v", err)
	}
	if s.State() != StateThinking {
		t.Fatalf("expected state=thinking after transition, got %s", s.State())
	}

	if err := s.TransitionTo(StateSpeaking); err != nil {
		t.Fatalf("transition thinking→speaking failed: %v", err)
	}
	if s.State() != StateSpeaking {
		t.Fatalf("expected state=speaking after transition, got %s", s.State())
	}

	if err := s.TransitionTo(StateIdle); err != nil {
		t.Fatalf("transition speaking→idle failed: %v", err)
	}
	if s.State() != StateIdle {
		t.Fatalf("expected state=idle after transition, got %s", s.State())
	}
}

func TestChatSession_InvalidTransition(t *testing.T) {
	s := NewChatSession("test-device", "test-session")

	if err := s.TransitionTo(StateSpeaking); err == nil {
		t.Fatal("expected error for idle→speaking, got nil")
	}
	if s.State() != StateIdle {
		t.Fatalf("state should remain idle after invalid transition, got %s", s.State())
	}

	s.TransitionTo(StateListening)
	if err := s.TransitionTo(StateSpeaking); err == nil {
		t.Fatal("expected error for listening→speaking, got nil")
	}
}

func TestChatSession_StateValues(t *testing.T) {
	if StateIdle != "idle" {
		t.Errorf("StateIdle = %q, want idle", StateIdle)
	}
	if StateListening != "listening" {
		t.Errorf("StateListening = %q, want listening", StateListening)
	}
	if StateThinking != "thinking" {
		t.Errorf("StateThinking = %q, want thinking", StateThinking)
	}
	if StateSpeaking != "speaking" {
		t.Errorf("StateSpeaking = %q, want speaking", StateSpeaking)
	}
}

func TestChatSession_Getters(t *testing.T) {
	s := NewChatSession("device-abc", "session-xyz")

	if s.ID() != "session-xyz" {
		t.Errorf("ID() = %q, want session-xyz", s.ID())
	}
	if s.DeviceID() != "device-abc" {
		t.Errorf("DeviceID() = %q, want device-abc", s.DeviceID())
	}

	if s.AudioBuffer() == nil {
		t.Fatal("AudioBuffer() returned nil")
	}

	before := s.LastActiveAt()
	time.Sleep(10 * time.Millisecond)
	s.Touch()
	after := s.LastActiveAt()
	if !after.After(before) {
		t.Error("LastActiveAt should be updated after Touch()")
	}
}
