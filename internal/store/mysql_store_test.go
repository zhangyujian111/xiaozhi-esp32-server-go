package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestMySQLMemory_SaveTurn(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mem := NewMySQLMemory(db, 10, 5, 0)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO xiaozhi_message").
		WithArgs("device1", "session1", "user", "hello").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO xiaozhi_message").
		WithArgs("device1", "session1", "assistant", "hi there").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	err = mem.SaveTurn(context.Background(), "device1", "session1", "hello", "hi there")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLMemory_GetWindow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mem := NewMySQLMemory(db, 10, 5, 0)

	rows := sqlmock.NewRows([]string{"id", "device_id", "session_id", "role", "content", "created_at"}).
		AddRow(1, "device1", "session1", "user", "hello", time.Now()).
		AddRow(2, "device1", "session1", "assistant", "hi there", time.Now())

	mock.ExpectQuery(`SELECT .+ FROM xiaozhi_message WHERE device_id = \? ORDER BY created_at ASC LIMIT \?`).
		WithArgs("device1", 20).
		WillReturnRows(rows)

	msgs, err := mem.GetWindow(context.Background(), "device1", 20)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	require.Equal(t, "user", msgs[0].Role)
	require.Equal(t, "hello", msgs[0].Content)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLMemory_GetWindow_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mem := NewMySQLMemory(db, 10, 5, 0)

	mock.ExpectQuery(`SELECT .+ FROM xiaozhi_message WHERE device_id = \?`).
		WithArgs("device1", 20).
		WillReturnError(errors.New("query error"))

	msgs, err := mem.GetWindow(context.Background(), "device1", 20)
	require.Error(t, err)
	require.Contains(t, err.Error(), "query error")
	require.Nil(t, msgs)
}

func TestMySQLMemory_SaveTurn_BeginError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mem := NewMySQLMemory(db, 10, 5, 0)

	mock.ExpectBegin().WillReturnError(errors.New("begin error"))

	err = mem.SaveTurn(context.Background(), "device1", "session1", "hello", "hi there")
	require.Error(t, err)
	require.Contains(t, err.Error(), "begin error")
}

func TestMySQLMemory_SaveTurn_ExecError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mem := NewMySQLMemory(db, 10, 5, 0)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO xiaozhi_message").
		WithArgs("device1", "session1", "user", "hello").
		WillReturnError(errors.New("exec error"))

	err = mem.SaveTurn(context.Background(), "device1", "session1", "hello", "hi there")
	require.Error(t, err)
	require.Contains(t, err.Error(), "exec error")
}

func TestMySQLMemory_SaveTurn_SecondExecError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mem := NewMySQLMemory(db, 10, 5, 0)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO xiaozhi_message").
		WithArgs("device1", "session1", "user", "hello").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO xiaozhi_message").
		WithArgs("device1", "session1", "assistant", "hi there").
		WillReturnError(errors.New("second exec error"))

	err = mem.SaveTurn(context.Background(), "device1", "session1", "hello", "hi there")
	require.Error(t, err)
	require.Contains(t, err.Error(), "second exec error")
}
