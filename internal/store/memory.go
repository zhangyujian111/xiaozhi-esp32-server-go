package store

import (
	"context"
	"time"
)

type Message struct {
	ID        int64
	DeviceID  string
	SessionID string
	Role      string
	Content   string
	CreatedAt time.Time
}

type Memory interface {
	GetWindow(ctx context.Context, deviceID string, n int) ([]Message, error)
	SaveTurn(ctx context.Context, deviceID, sessionID, userText, assistantText string) error
}
