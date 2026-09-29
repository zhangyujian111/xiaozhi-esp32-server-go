package store

import (
	"context"
	"sort"
	"sync"
	"time"
)

type InMemoryMemory struct {
	mu       sync.Mutex
	messages []Message
}

func NewInMemoryMemory() *InMemoryMemory {
	return &InMemoryMemory{}
}

func (m *InMemoryMemory) GetWindow(ctx context.Context, deviceID string, n int) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var filtered []Message
	for _, msg := range m.messages {
		if msg.DeviceID == deviceID {
			filtered = append(filtered, msg)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})

	if len(filtered) > n {
		return filtered[len(filtered)-n:], nil
	}
	return filtered, nil
}

func (m *InMemoryMemory) SaveTurn(ctx context.Context, deviceID, sessionID, userText, assistantText string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.messages = append(m.messages, Message{
		DeviceID:  deviceID,
		SessionID: sessionID,
		Role:      "user",
		Content:   userText,
		CreatedAt: now,
	}, Message{
		DeviceID:  deviceID,
		SessionID: sessionID,
		Role:      "assistant",
		Content:   assistantText,
		CreatedAt: now,
	})
	return nil
}
