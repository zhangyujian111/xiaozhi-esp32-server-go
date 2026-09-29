package ws

import (
	"fmt"
	"sync"
	"time"
)

type SessionState string

const (
	StateIdle      SessionState = "idle"
	StateListening SessionState = "listening"
	StateThinking  SessionState = "thinking"
	StateSpeaking  SessionState = "speaking"
)

var validTransitions = map[SessionState][]SessionState{
	StateIdle:      {StateListening},
	StateListening: {StateThinking, StateIdle},
	StateThinking:  {StateSpeaking, StateIdle},
	StateSpeaking:  {StateListening, StateIdle},
}

type ChatSession struct {
	mu           sync.RWMutex
	id           string
	deviceID     string
	state        SessionState
	createdAt    time.Time
	lastActiveAt time.Time
	audioBuf     *AudioRingBuffer
}

func NewChatSession(deviceID, sessionID string) *ChatSession {
	now := time.Now()
	return &ChatSession{
		id:           sessionID,
		deviceID:     deviceID,
		state:        StateIdle,
		createdAt:    now,
		lastActiveAt: now,
		audioBuf:     NewAudioRingBuffer(16000, 60*5),
	}
}

func (s *ChatSession) ID() string       { return s.id }
func (s *ChatSession) DeviceID() string { return s.deviceID }

func (s *ChatSession) State() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *ChatSession) TransitionTo(target SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, allowed := range validTransitions[s.state] {
		if allowed == target {
			s.state = target
			s.lastActiveAt = time.Now()
			return nil
		}
	}
	return fmt.Errorf("invalid transition %s → %s", s.state, target)
}

func (s *ChatSession) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActiveAt = time.Now()
}

func (s *ChatSession) AudioBuffer() *AudioRingBuffer { return s.audioBuf }

func (s *ChatSession) LastActiveAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastActiveAt
}
