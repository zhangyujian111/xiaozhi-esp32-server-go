package obs

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

func InitLogger(level string, format string) zerolog.Logger {
	zerologLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		zerologLevel = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(zerologLevel)
	zerolog.TimeFieldFormat = time.RFC3339

	// Tee stdout 到 ./bin/xiaozhi-server.log（不依赖 cmd /c 嵌套重定向，
	// 即便启动方式（start /min / PowerShell Start-Process）丢 stdout，本地仍可看日志）。
	var writer io.Writer = os.Stdout
	if logFile, ferr := openServerLog(); ferr == nil {
		writer = zerolog.MultiLevelWriter(os.Stdout, logFile)
	}

	var logger zerolog.Logger
	if format == "console" {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: writer}).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(writer).With().Timestamp().Logger()
	}

	logger = logger.Level(zerologLevel)

	return logger
}

func openServerLog() (io.Writer, error) {
	logPath := filepath.Join("logs", "xiaozhi-server.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return f, nil
}
