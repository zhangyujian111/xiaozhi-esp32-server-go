package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInMemoryMemory_GetWindow_Empty(t *testing.T) {
	mem := NewInMemoryMemory()
	msgs, err := mem.GetWindow(context.Background(), "device1", 10)
	require.NoError(t, err)
	require.Empty(t, msgs)
}

func TestInMemoryMemory_SaveTurn_GetWindow(t *testing.T) {
	mem := NewInMemoryMemory()
	ctx := context.Background()

	err := mem.SaveTurn(ctx, "device1", "session1", "hello", "hi there")
	require.NoError(t, err)

	err = mem.SaveTurn(ctx, "device1", "session1", "how are you", "i am good")
	require.NoError(t, err)

	err = mem.SaveTurn(ctx, "device1", "session1", "third turn", "response three")
	require.NoError(t, err)

	msgs, err := mem.GetWindow(ctx, "device1", 4)
	require.NoError(t, err)
	require.Len(t, msgs, 4)

	require.Equal(t, "user", msgs[0].Role)
	require.Equal(t, "how are you", msgs[0].Content)
	require.Equal(t, "assistant", msgs[1].Role)
	require.Equal(t, "i am good", msgs[1].Content)
	require.Equal(t, "user", msgs[2].Role)
	require.Equal(t, "third turn", msgs[2].Content)
	require.Equal(t, "assistant", msgs[3].Role)
	require.Equal(t, "response three", msgs[3].Content)
}

func TestInMemoryMemory_GetWindow_OrderedByTime(t *testing.T) {
	mem := NewInMemoryMemory()
	ctx := context.Background()

	mem.messages = []Message{
		{DeviceID: "d1", SessionID: "s1", Role: "user", Content: "first", CreatedAt: time.Now().Add(-2 * time.Hour)},
		{DeviceID: "d1", SessionID: "s1", Role: "assistant", Content: "first resp", CreatedAt: time.Now().Add(-2 * time.Hour)},
		{DeviceID: "d1", SessionID: "s1", Role: "user", Content: "second", CreatedAt: time.Now().Add(-1 * time.Hour)},
		{DeviceID: "d1", SessionID: "s1", Role: "assistant", Content: "second resp", CreatedAt: time.Now().Add(-1 * time.Hour)},
	}

	msgs, err := mem.GetWindow(ctx, "d1", 10)
	require.NoError(t, err)
	require.Len(t, msgs, 4)
	require.Equal(t, "first", msgs[0].Content)
	require.Equal(t, "second resp", msgs[3].Content)
}
