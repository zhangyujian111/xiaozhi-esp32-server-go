package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileDeviceStore 持久化 device store（JSON 文件版）。
//
// 用途：解决 InMemoryDeviceStore 在 xz-server 重启后丢失 aisaas APIKey 导致
// STT/LLM/TTS 调用 401 的问题。文件存到 ./data/devices.json，重启自动加载。
//
// 仅实现 DeviceStore 接口 + 原子落盘；并发安全（RWMutex）。
// 不替代生产级 MySQL store（MysqlDeviceStore），仅供单机开发/演示。
type FileDeviceStore struct {
	mu       sync.RWMutex
	filePath string
	devices  map[string]*Device
}

func NewFileDeviceStore(filePath string) (*FileDeviceStore, error) {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, err
	}
	s := &FileDeviceStore{
		filePath: filePath,
		devices:  make(map[string]*Device),
	}
	if err := s.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s, nil
}

type fileDeviceRecord struct {
	DeviceID          string `json:"device_id"`
	UserID            int64  `json:"user_id"`
	ActivationVersion int    `json:"activation_version"`
	ClientID          string `json:"client_id"`
	SerialNumber      string `json:"serial_number"`
	FirmwareVersion   string `json:"firmware_version"`
	LastSeenAt        int64  `json:"last_seen_at"`
	ActivatedAt       int64  `json:"activated_at"`
	Token             string `json:"token"`
	APIKey            string `json:"api_key"`
	CreatedAt         int64  `json:"created_at"`
}

func (s *FileDeviceStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var records []fileDeviceRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return err
	}
	for i := range records {
		r := &records[i]
		s.devices[r.DeviceID] = &Device{
			DeviceID:          r.DeviceID,
			UserID:            r.UserID,
			ActivationVersion: r.ActivationVersion,
			ClientID:          r.ClientID,
			SerialNumber:      r.SerialNumber,
			FirmwareVersion:   r.FirmwareVersion,
			LastSeenAt:        timeFromUnix(r.LastSeenAt),
			ActivatedAt:       timeFromUnix(r.ActivatedAt),
			Token:             r.Token,
			APIKey:            r.APIKey,
			CreatedAt:         timeFromUnix(r.CreatedAt),
		}
	}
	return nil
}

func (s *FileDeviceStore) persist() error {
	records := make([]fileDeviceRecord, 0, len(s.devices))
	for _, d := range s.devices {
		records = append(records, fileDeviceRecord{
			DeviceID:          d.DeviceID,
			UserID:            d.UserID,
			ActivationVersion: d.ActivationVersion,
			ClientID:          d.ClientID,
			SerialNumber:      d.SerialNumber,
			FirmwareVersion:   d.FirmwareVersion,
			LastSeenAt:        d.LastSeenAt.Unix(),
			ActivatedAt:       d.ActivatedAt.Unix(),
			Token:             d.Token,
			APIKey:            d.APIKey,
			CreatedAt:         d.CreatedAt.Unix(),
		})
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath)
}

func timeFromUnix(t int64) time.Time {
	if t <= 0 {
		return time.Time{}
	}
	return time.Unix(t, 0)
}

func (s *FileDeviceStore) GetDevice(ctx context.Context, deviceID string) (*Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dev, ok := s.devices[deviceID]
	if !ok {
		return nil, ErrDeviceNotFound
	}
	cp := *dev
	return &cp, nil
}

func (s *FileDeviceStore) UpsertDevice(ctx context.Context, device *Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[device.DeviceID]; !ok {
		device.CreatedAt = time.Now()
	}
	cp := *device
	s.devices[device.DeviceID] = &cp
	return s.persist()
}

func (s *FileDeviceStore) ListDevices(ctx context.Context, limit, offset int) ([]Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, *d)
	}
	if offset >= len(out) {
		return []Device{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (s *FileDeviceStore) ActivateDevice(ctx context.Context, deviceID, clientID, serialNumber string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dev, ok := s.devices[deviceID]
	if !ok {
		return ErrDeviceNotFound
	}
	dev.ActivationVersion++
	dev.ClientID = clientID
	dev.SerialNumber = serialNumber
	dev.ActivatedAt = time.Now()
	dev.LastSeenAt = time.Now()
	cp := *dev
	s.devices[deviceID] = &cp
	return s.persist()
}

func (s *FileDeviceStore) UpdateDevice(ctx context.Context, device *Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[device.DeviceID]; !ok {
		return ErrDeviceNotFound
	}
	cp := *device
	s.devices[device.DeviceID] = &cp
	return s.persist()
}

func (s *FileDeviceStore) DeleteDevice(ctx context.Context, deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[deviceID]; !ok {
		return ErrDeviceNotFound
	}
	delete(s.devices, deviceID)
	return s.persist()
}

func (s *FileDeviceStore) BindDevice(ctx context.Context, deviceID string, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dev, ok := s.devices[deviceID]
	if !ok {
		return ErrDeviceNotFound
	}
	dev.UserID = userID
	cp := *dev
	s.devices[deviceID] = &cp
	return s.persist()
}

func (s *FileDeviceStore) UnbindDevice(ctx context.Context, deviceID string) error {
	return s.BindDevice(ctx, deviceID, 0)
}

var _ DeviceStore = (*FileDeviceStore)(nil)