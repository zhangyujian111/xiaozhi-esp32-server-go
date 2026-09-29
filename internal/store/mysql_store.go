package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var ErrNotImplemented = errors.New("not implemented")

type MySQLMemory struct {
	db              *sql.DB
	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
}

func NewMySQLMemory(db *sql.DB, maxOpenConns, maxIdleConns int, connMaxLifetime time.Duration) *MySQLMemory {
	return &MySQLMemory{
		db:              db,
		maxOpenConns:    maxOpenConns,
		maxIdleConns:    maxIdleConns,
		connMaxLifetime: connMaxLifetime,
	}
}

func NewMySQLFromConfig(db *sql.DB, cfg struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}) *MySQLMemory {
	return NewMySQLMemory(db, cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxLifetime)
}

func (m *MySQLMemory) GetWindow(ctx context.Context, deviceID string, n int) ([]Message, error) {
	query := `
		SELECT id, device_id, session_id, role, content, created_at
		FROM xiaozhi_message
		WHERE device_id = ?
		ORDER BY created_at ASC
		LIMIT ?
	`
	rows, err := m.db.QueryContext(ctx, query, deviceID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.DeviceID, &msg.SessionID, &msg.Role, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (m *MySQLMemory) SaveTurn(ctx context.Context, deviceID, sessionID, userText, assistantText string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	insertQuery := `INSERT INTO xiaozhi_message (device_id, session_id, role, content) VALUES (?, ?, ?, ?)`
	_, err = tx.ExecContext(ctx, insertQuery, deviceID, sessionID, "user", userText)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, insertQuery, deviceID, sessionID, "assistant", assistantText)
	if err != nil {
		return err
	}

	return tx.Commit()
}
