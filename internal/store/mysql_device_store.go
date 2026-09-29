package store

import (
	"context"
	"database/sql"
	"time"
)

type MySQLDeviceStore struct {
	db *sql.DB
}

func NewMySQLDeviceStore(db *sql.DB) *MySQLDeviceStore {
	return &MySQLDeviceStore{db: db}
}

func (s *MySQLDeviceStore) GetDevice(ctx context.Context, deviceID string) (*Device, error) {
	query := `SELECT device_id, user_id, activation_version, client_id, serial_number,
		firmware_version, last_seen_at, activated_at, token, created_at
		FROM xiaozhi_device WHERE device_id = ?`
	var d Device
	err := s.db.QueryRowContext(ctx, query, deviceID).Scan(
		&d.DeviceID, &d.UserID, &d.ActivationVersion, &d.ClientID, &d.SerialNumber,
		&d.FirmwareVersion, &d.LastSeenAt, &d.ActivatedAt, &d.Token, &d.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrDeviceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *MySQLDeviceStore) UpsertDevice(ctx context.Context, device *Device) error {
	query := `INSERT INTO xiaozhi_device (device_id, user_id, firmware_version, token, last_seen_at)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), firmware_version=VALUES(firmware_version),
		token=VALUES(token), last_seen_at=VALUES(last_seen_at)`
	now := time.Now()
	if device.CreatedAt.IsZero() {
		device.CreatedAt = now
	}
	_, err := s.db.ExecContext(ctx, query, device.DeviceID, device.UserID,
		device.FirmwareVersion, device.Token, now)
	return err
}

func (s *MySQLDeviceStore) ListDevices(ctx context.Context, limit, offset int) ([]Device, error) {
	query := `SELECT device_id, user_id, activation_version, client_id, serial_number,
		firmware_version, last_seen_at, activated_at, token, created_at
		FROM xiaozhi_device ORDER BY device_id LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.DeviceID, &d.UserID, &d.ActivationVersion, &d.ClientID,
			&d.SerialNumber, &d.FirmwareVersion, &d.LastSeenAt, &d.ActivatedAt,
			&d.Token, &d.CreatedAt); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *MySQLDeviceStore) ActivateDevice(ctx context.Context, deviceID, clientID, serialNumber string) error {
	query := `UPDATE xiaozhi_device SET activation_version = activation_version + 1,
		client_id = ?, serial_number = ?, activated_at = NOW(), last_seen_at = NOW()
		WHERE device_id = ?`
	result, err := s.db.ExecContext(ctx, query, clientID, serialNumber, deviceID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *MySQLDeviceStore) UpdateDevice(ctx context.Context, device *Device) error {
	query := `UPDATE xiaozhi_device SET user_id=?, client_id=?, serial_number=?,
		firmware_version=?, last_seen_at=? WHERE device_id=?`
	result, err := s.db.ExecContext(ctx, query, device.UserID, device.ClientID,
		device.SerialNumber, device.FirmwareVersion, time.Now(), device.DeviceID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *MySQLDeviceStore) DeleteDevice(ctx context.Context, deviceID string) error {
	query := `DELETE FROM xiaozhi_device WHERE device_id = ?`
	result, err := s.db.ExecContext(ctx, query, deviceID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *MySQLDeviceStore) BindDevice(ctx context.Context, deviceID string, userID int64) error {
	query := `UPDATE xiaozhi_device SET user_id = ? WHERE device_id = ?`
	result, err := s.db.ExecContext(ctx, query, userID, deviceID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *MySQLDeviceStore) UnbindDevice(ctx context.Context, deviceID string) error {
	return s.BindDevice(ctx, deviceID, 0)
}

var _ DeviceStore = (*MySQLDeviceStore)(nil)
