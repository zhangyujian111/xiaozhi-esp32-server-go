package store

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestMySQLDeviceStore_GetDevice_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewMySQLDeviceStore(db)
	now := time.Now()
	cols := []string{"device_id", "user_id", "activation_version", "client_id", "serial_number",
		"firmware_version", "last_seen_at", "activated_at", "token", "created_at"}
	mock.ExpectQuery("SELECT .+ FROM xiaozhi_device WHERE device_id = ?").
		WithArgs("dev1").
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			"dev1", int64(100), 2, "clientA", "SN123", "1.0.0", now, now, "token1", now))

	dev, err := store.GetDevice(context.Background(), "dev1")
	require.NoError(t, err)
	require.Equal(t, "dev1", dev.DeviceID)
	require.Equal(t, int64(100), dev.UserID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLDeviceStore_GetDevice_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewMySQLDeviceStore(db)
	mock.ExpectQuery("SELECT .+ FROM xiaozhi_device WHERE device_id = ?").
		WithArgs("nonexistent").
		WillReturnRows(sqlmock.NewRows([]string{"device_id", "user_id", "activation_version",
			"client_id", "serial_number", "firmware_version", "last_seen_at", "activated_at",
			"token", "created_at"}))

	_, err = store.GetDevice(context.Background(), "nonexistent")
	require.ErrorIs(t, err, ErrDeviceNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLDeviceStore_UpsertDevice_New(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewMySQLDeviceStore(db)
	device := &Device{DeviceID: "newdev", UserID: 1, Token: "tok", FirmwareVersion: "1.0.0"}
	mock.ExpectExec("INSERT INTO xiaozhi_device .+ ON DUPLICATE KEY UPDATE").
		WithArgs("newdev", int64(1), "1.0.0", "tok", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = store.UpsertDevice(context.Background(), device)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLDeviceStore_ActivateDevice_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewMySQLDeviceStore(db)
	mock.ExpectExec("UPDATE xiaozhi_device SET activation_version = .+ WHERE device_id = ?").
		WithArgs("clientA", "SN123", "dev1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.ActivateDevice(context.Background(), "dev1", "clientA", "SN123")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLDeviceStore_ActivateDevice_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewMySQLDeviceStore(db)
	mock.ExpectExec("UPDATE xiaozhi_device SET activation_version = .+ WHERE device_id = ?").
		WithArgs("clientA", "SN123", "nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = store.ActivateDevice(context.Background(), "nonexistent", "clientA", "SN123")
	require.ErrorIs(t, err, ErrDeviceNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLDeviceStore_DeleteDevice_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewMySQLDeviceStore(db)
	mock.ExpectExec("DELETE FROM xiaozhi_device WHERE device_id = ?").
		WithArgs("dev1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.DeleteDevice(context.Background(), "dev1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLDeviceStore_DeleteDevice_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewMySQLDeviceStore(db)
	mock.ExpectExec("DELETE FROM xiaozhi_device WHERE device_id = ?").
		WithArgs("nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = store.DeleteDevice(context.Background(), "nonexistent")
	require.ErrorIs(t, err, ErrDeviceNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLDeviceStore_BindDevice_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewMySQLDeviceStore(db)
	mock.ExpectExec("UPDATE xiaozhi_device SET user_id = \\? WHERE device_id = \\?").
		WithArgs(int64(42), "nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = store.BindDevice(context.Background(), "nonexistent", 42)
	require.ErrorIs(t, err, ErrDeviceNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLDeviceStore_BindDevice_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewMySQLDeviceStore(db)
	mock.ExpectExec("UPDATE xiaozhi_device SET user_id = \\? WHERE device_id = \\?").
		WithArgs(int64(42), "dev1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.BindDevice(context.Background(), "dev1", 42)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLDeviceStore_UnbindDevice(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewMySQLDeviceStore(db)
	mock.ExpectExec("UPDATE xiaozhi_device SET user_id = \\? WHERE device_id = \\?").
		WithArgs(int64(0), "dev1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.UnbindDevice(context.Background(), "dev1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLDeviceStore_ListDevices(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewMySQLDeviceStore(db)
	now := time.Now()
	cols := []string{"device_id", "user_id", "activation_version", "client_id", "serial_number",
		"firmware_version", "last_seen_at", "activated_at", "token", "created_at"}
	mock.ExpectQuery("SELECT .+ FROM xiaozhi_device ORDER BY device_id LIMIT \\? OFFSET \\?").
		WithArgs(10, 0).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow("dev1", int64(1), 1, "c1", "s1", "1.0.0", now, now, "t1", now).
			AddRow("dev2", int64(2), 1, "c2", "s2", "1.0.0", now, now, "t2", now))

	devs, err := store.ListDevices(context.Background(), 10, 0)
	require.NoError(t, err)
	require.Len(t, devs, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLDeviceStore_UpdateDevice_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := NewMySQLDeviceStore(db)
	mock.ExpectExec("UPDATE xiaozhi_device SET user_id=.+ WHERE device_id=?").
		WithArgs(int64(999), "clientX", "SNX", "2.0.0", sqlmock.AnyArg(), "dev1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = s.UpdateDevice(context.Background(), &Device{
		DeviceID: "dev1", UserID: 999, ClientID: "clientX", SerialNumber: "SNX", FirmwareVersion: "2.0.0",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLDeviceStore_UpdateDevice_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := NewMySQLDeviceStore(db)
	mock.ExpectExec("UPDATE xiaozhi_device SET user_id=.+ WHERE device_id=?").
		WithArgs(int64(999), "clientX", "SNX", "2.0.0", sqlmock.AnyArg(), "nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = s.UpdateDevice(context.Background(), &Device{
		DeviceID: "nonexistent", UserID: 999, ClientID: "clientX", SerialNumber: "SNX", FirmwareVersion: "2.0.0",
	})
	require.ErrorIs(t, err, ErrDeviceNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
