package obs

import (
	"os"
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

	var logger zerolog.Logger
	if format == "console" {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	logger = logger.Level(zerologLevel)

	return logger
}
