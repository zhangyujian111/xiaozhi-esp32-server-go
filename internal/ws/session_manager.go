package ws

import (
	"fmt"
	"sync"
)

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*ChatSession
	byDevice map[string]string
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*ChatSession),
		byDevice: make(map[string]string),
	}
}

func (m *SessionManager) Register(s *ChatSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.byDevice[s.DeviceID()]; ok {
		return fmt.Errorf("device %s already has session %s", s.DeviceID(), existing)
	}
	m.sessions[s.ID()] = s
	m.byDevice[s.DeviceID()] = s.ID()
	return nil
}

func (m *SessionManager) Get(sessionID string) (*ChatSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[sessionID]
	return s, ok
}

func (m *SessionManager) GetByDevice(deviceID string) (*ChatSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sid, ok := m.byDevice[deviceID]
	if !ok {
		return nil, false
	}
	return m.sessions[sid], true
}

func (m *SessionManager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		delete(m.byDevice, s.DeviceID())
		delete(m.sessions, sessionID)
	}
}

func (m *SessionManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}
