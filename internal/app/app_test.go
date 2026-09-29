package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
)

func TestNewApp_Success(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			WebsocketAddr: "localhost:18080",
			AdminAddr:     "localhost:18081",
			ReadTimeout:   "30s",
			WriteTimeout:  "30s",
			MaxConns:      100,
			InternalToken: "test-token",
		},
		Database: config.DatabaseConfig{
			DSN:             "test:test@tcp(localhost:3306)/test?parseTime=true",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: "1h",
		},
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "console",
		},
		Aisaas: config.AisaasConfig{
			BaseURL:       "http://localhost:9999",
			InternalToken: "test-internal-token",
			HTTPTimeout:   "10s",
		},
		VAD: config.VADConfig{
			SpeechThreshold:   0.5,
			SilenceThreshold:  0.5,
			SilenceDurationMs: 500,
			FrameSizeSamples:  480,
		},
		Opus: config.OpusConfig{
			Uplink:   config.OpusDirection{SampleRate: 16000, Channels: 1, FrameDurationMs: 60},
			Downlink: config.OpusDirection{SampleRate: 24000, Channels: 1, FrameDurationMs: 60},
		},
		Dialogue: config.DialogueConfig{
			PerTurnTimeoutSec:    30,
			WindowMemorySize:     10,
			TTFSAlertThresholdMs: 1000,
		},
	}

	app, err := NewApp(cfg)

	require.NoError(t, err)
	require.NotNil(t, app)
	require.NotNil(t, app.cfg)
	require.NotNil(t, app.deviceStore)
	require.NotNil(t, app.memory)
	require.NotNil(t, app.eventBus)
	require.NotNil(t, app.opusDecoder)
	require.NotNil(t, app.opusEncoder)
	require.NotNil(t, app.aisaasClient)
	require.NotNil(t, app.wsHandler)
}

func TestNewApp_InvalidConfig_ValidationError(t *testing.T) {
	cfg := &config.Config{}

	app, err := NewApp(cfg)

	require.Error(t, err)
	require.Nil(t, app)
}

func TestApp_Shutdown_Idempotent(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			WebsocketAddr: "localhost:18080",
			AdminAddr:     "localhost:18081",
			ReadTimeout:   "30s",
			WriteTimeout:  "30s",
			MaxConns:      100,
			InternalToken: "test-token",
		},
		Database: config.DatabaseConfig{
			DSN:             "test:test@tcp(localhost:3306)/test?parseTime=true",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: "1h",
		},
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "console",
		},
		Aisaas: config.AisaasConfig{
			BaseURL:       "http://localhost:9999",
			InternalToken: "test-internal-token",
			HTTPTimeout:   "10s",
		},
		VAD: config.VADConfig{
			SpeechThreshold:   0.5,
			SilenceThreshold:  0.5,
			SilenceDurationMs: 500,
			FrameSizeSamples:  480,
		},
		Opus: config.OpusConfig{
			Uplink:   config.OpusDirection{SampleRate: 16000, Channels: 1, FrameDurationMs: 60},
			Downlink: config.OpusDirection{SampleRate: 24000, Channels: 1, FrameDurationMs: 60},
		},
		Dialogue: config.DialogueConfig{
			PerTurnTimeoutSec:    30,
			WindowMemorySize:     10,
			TTFSAlertThresholdMs: 1000,
		},
	}

	app, err := NewApp(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	err = app.Shutdown(ctx)
	require.NoError(t, err)

	err = app.Shutdown(ctx)
	require.NoError(t, err)
}
