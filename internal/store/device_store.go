package store

import (
	"context"
	"errors"
	"sort"
	"time"
)

var ErrDeviceNotFound = errors.New("device not found")

type Device struct {
	DeviceID          string
	UserID            int64
	ActivationVersion int
	ClientID          string
	SerialNumber      string
	FirmwareVersion   string
	LastSeenAt        time.Time
	ActivatedAt       time.Time
	Token             string
	CreatedAt         time.Time
}

type DeviceStore interface {
	GetDevice(ctx context.Context, deviceID string) (*Device, error)
	UpsertDevice(ctx context.Context, device *Device) error
	ListDevices(ctx context.Context, limit, offset int) ([]Device, error)
	ActivateDevice(ctx context.Context, deviceID, clientID, serialNumber string) error
	UpdateDevice(ctx context.Context, device *Device) error
	DeleteDevice(ctx context.Context, deviceID string) error
	BindDevice(ctx context.Context, deviceID string, userID int64) error
	UnbindDevice(ctx context.Context, deviceID string) error
}

type InMemoryDeviceStore struct {
	devices map[string]*Device
}

func NewInMemoryDeviceStore() *InMemoryDeviceStore {
	return &InMemoryDeviceStore{
		devices: make(map[string]*Device),
	}
}

func (s *InMemoryDeviceStore) GetDevice(ctx context.Context, deviceID string) (*Device, error) {
	if dev, ok := s.devices[deviceID]; ok {
		return dev, nil
	}
	return nil, ErrDeviceNotFound
}

func (s *InMemoryDeviceStore) UpsertDevice(ctx context.Context, device *Device) error {
	if _, ok := s.devices[device.DeviceID]; !ok {
		device.CreatedAt = time.Now()
	}
	s.devices[device.DeviceID] = device
	return nil
}

func (s *InMemoryDeviceStore) ListDevices(ctx context.Context, limit, offset int) ([]Device, error) {
	ordered := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		ordered = append(ordered, *d)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].DeviceID < ordered[j].DeviceID })
	if offset >= len(ordered) {
		return []Device{}, nil
	}
	end := offset + limit
	if end > len(ordered) {
		end = len(ordered)
	}
	return ordered[offset:end], nil
}

func (s *InMemoryDeviceStore) ActivateDevice(ctx context.Context, deviceID, clientID, serialNumber string) error {
	dev, ok := s.devices[deviceID]
	if !ok {
		return ErrDeviceNotFound
	}
	dev.ActivationVersion++
	dev.ClientID = clientID
	dev.SerialNumber = serialNumber
	dev.ActivatedAt = time.Now()
	dev.LastSeenAt = time.Now()
	return nil
}

func (s *InMemoryDeviceStore) UpdateDevice(ctx context.Context, device *Device) error {
	if _, ok := s.devices[device.DeviceID]; !ok {
		return ErrDeviceNotFound
	}
	s.devices[device.DeviceID] = device
	return nil
}

func (s *InMemoryDeviceStore) DeleteDevice(ctx context.Context, deviceID string) error {
	if _, ok := s.devices[deviceID]; !ok {
		return ErrDeviceNotFound
	}
	delete(s.devices, deviceID)
	return nil
}

func (s *InMemoryDeviceStore) BindDevice(ctx context.Context, deviceID string, userID int64) error {
	dev, ok := s.devices[deviceID]
	if !ok {
		return ErrDeviceNotFound
	}
	dev.UserID = userID
	return nil
}

func (s *InMemoryDeviceStore) UnbindDevice(ctx context.Context, deviceID string) error {
	dev, ok := s.devices[deviceID]
	if !ok {
		return ErrDeviceNotFound
	}
	dev.UserID = 0
	return nil
}

var _ DeviceStore = (*InMemoryDeviceStore)(nil)
