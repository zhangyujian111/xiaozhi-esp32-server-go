package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInMemoryDeviceStore_GetDevice_NotFound(t *testing.T) {
	s := NewInMemoryDeviceStore()
	_, err := s.GetDevice(context.Background(), "nonexistent")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDeviceNotFound)
}

func TestInMemoryDeviceStore_GetDevice_Found(t *testing.T) {
	s := NewInMemoryDeviceStore()
	now := time.Now()
	s.devices["dev1"] = &Device{
		DeviceID:  "dev1",
		UserID:    100,
		Token:     "token1",
		CreatedAt: now,
	}
	got, err := s.GetDevice(context.Background(), "dev1")
	require.NoError(t, err)
	require.Equal(t, "dev1", got.DeviceID)
	require.Equal(t, int64(100), got.UserID)
}

func TestInMemoryDeviceStore_UpsertDevice_New(t *testing.T) {
	s := NewInMemoryDeviceStore()
	device := &Device{DeviceID: "newdev", UserID: 1}
	err := s.UpsertDevice(context.Background(), device)
	require.NoError(t, err)
	got, err := s.GetDevice(context.Background(), "newdev")
	require.NoError(t, err)
	require.Equal(t, "newdev", got.DeviceID)
}

func TestInMemoryDeviceStore_UpsertDevice_Update(t *testing.T) {
	s := NewInMemoryDeviceStore()
	device := &Device{DeviceID: "dev", UserID: 1, Token: "old"}
	s.devices["dev"] = device
	updated := &Device{DeviceID: "dev", UserID: 2, Token: "new"}
	err := s.UpsertDevice(context.Background(), updated)
	require.NoError(t, err)
	require.Equal(t, int64(2), s.devices["dev"].UserID)
	require.Equal(t, "new", s.devices["dev"].Token)
}

func TestInMemoryDeviceStore_ListDevices(t *testing.T) {
	s := NewInMemoryDeviceStore()
	for i := 0; i < 5; i++ {
		s.devices[string(rune('a'+i))] = &Device{DeviceID: string(rune('a' + i))}
	}
	devs, err := s.ListDevices(context.Background(), 3, 1)
	require.NoError(t, err)
	require.Len(t, devs, 3)
}

func TestInMemoryDeviceStore_ActivateDevice(t *testing.T) {
	s := NewInMemoryDeviceStore()
	s.devices["dev1"] = &Device{DeviceID: "dev1", ActivationVersion: 0}
	err := s.ActivateDevice(context.Background(), "dev1", "clientA", "SN123")
	require.NoError(t, err)
	dev := s.devices["dev1"]
	require.Equal(t, 1, dev.ActivationVersion)
	require.Equal(t, "clientA", dev.ClientID)
	require.Equal(t, "SN123", dev.SerialNumber)
	require.False(t, dev.ActivatedAt.IsZero())
}

func TestInMemoryDeviceStore_UpdateDevice(t *testing.T) {
	s := NewInMemoryDeviceStore()
	s.devices["dev1"] = &Device{DeviceID: "dev1", UserID: 1}
	err := s.UpdateDevice(context.Background(), &Device{DeviceID: "dev1", UserID: 999})
	require.NoError(t, err)
	require.Equal(t, int64(999), s.devices["dev1"].UserID)
}

func TestInMemoryDeviceStore_UpdateDevice_NotFound(t *testing.T) {
	s := NewInMemoryDeviceStore()
	err := s.UpdateDevice(context.Background(), &Device{DeviceID: "nonexistent"})
	require.Error(t, err)
}

func TestInMemoryDeviceStore_DeleteDevice(t *testing.T) {
	s := NewInMemoryDeviceStore()
	s.devices["dev1"] = &Device{DeviceID: "dev1"}
	err := s.DeleteDevice(context.Background(), "dev1")
	require.NoError(t, err)
	_, exists := s.devices["dev1"]
	require.False(t, exists)
}

func TestInMemoryDeviceStore_DeleteDevice_NotFound(t *testing.T) {
	s := NewInMemoryDeviceStore()
	err := s.DeleteDevice(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestInMemoryDeviceStore_BindDevice(t *testing.T) {
	s := NewInMemoryDeviceStore()
	s.devices["dev1"] = &Device{DeviceID: "dev1", UserID: 0}
	err := s.BindDevice(context.Background(), "dev1", 42)
	require.NoError(t, err)
	require.Equal(t, int64(42), s.devices["dev1"].UserID)
}

func TestInMemoryDeviceStore_UnbindDevice(t *testing.T) {
	s := NewInMemoryDeviceStore()
	s.devices["dev1"] = &Device{DeviceID: "dev1", UserID: 42}
	err := s.UnbindDevice(context.Background(), "dev1")
	require.NoError(t, err)
	require.Equal(t, int64(0), s.devices["dev1"].UserID)
}

func TestInMemoryDeviceStore_BindDevice_NotFound(t *testing.T) {
	s := NewInMemoryDeviceStore()
	err := s.BindDevice(context.Background(), "nonexistent", 42)
	require.Error(t, err)
}

func TestInMemoryDeviceStore_UnbindDevice_NotFound(t *testing.T) {
	s := NewInMemoryDeviceStore()
	err := s.UnbindDevice(context.Background(), "nonexistent")
	require.Error(t, err)
}
