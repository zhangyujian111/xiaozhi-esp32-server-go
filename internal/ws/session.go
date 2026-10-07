package ws

import (
	"fmt"
	"sync"
	"time"

	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
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

// PcmDumper interface for debugging PCM dumps (used by debug dumpFile feature).
type PcmDumper interface {
	WriteChunk(chunk []int16)
}

type ChatSession struct {
	mu           sync.RWMutex
	id           string
	deviceID     string
	device       *aisaas.DeviceInfo // populated at HandleUpgrade via aisaas.GetDevice
	state        SessionState
	createdAt    time.Time
	lastActiveAt time.Time
	audioBuf     *AudioRingBuffer
	dumpFile     PcmDumper // nil unless debug PCM dump is enabled
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

// Device returns the aisaas device info for this session (may be nil if GetDevice failed at upgrade).
func (s *ChatSession) Device() *aisaas.DeviceInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.device
}

// SetDevice stores the device info (called by Handler.HandleUpgrade after aisaas.GetDevice).
func (s *ChatSession) SetDevice(d *aisaas.DeviceInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.device = d
}

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
