package config

import (
	"testing"
)

func TestValidate_RequiredFields(t *testing.T) {
	cfg := &Config{}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for empty config, got nil")
	}
}

func TestValidate_DuplicatePorts(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			WebsocketAddr: ":8080",
			AdminAddr:     ":8080",
			ReadTimeout:   "30s",
			WriteTimeout:  "30s",
			MaxConns:      200,
		},
		Aisaas: AisaasConfig{
			BaseURL:       "http://localhost:8190",
			InternalToken: "test-token",
			HTTPTimeout:   "30s",
		},
		VAD: VADConfig{
			SpeechThreshold:  0.5,
			SilenceThreshold: 0.3,
			FrameSizeSamples: 512,
		},
		Opus: OpusConfig{
			Uplink: OpusDirection{
				SampleRate: 16000,
				Channels:   1,
			},
			Downlink: OpusDirection{
				SampleRate: 24000,
				Channels:   1,
			},
		},
		Dialogue: DialogueConfig{
			PerTurnTimeoutSec: 30,
			WindowMemorySize:  20,
		},
		Database: DatabaseConfig{
			DSN:          "postgres://localhost/db",
			MaxOpenConns: 50,
			MaxIdleConns: 10,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for duplicate ports, got nil")
	}
}

func TestValidate_VADThresholdRange(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			WebsocketAddr: ":8080",
			AdminAddr:     ":8081",
			ReadTimeout:   "30s",
			WriteTimeout:  "30s",
			MaxConns:      200,
		},
		Aisaas: AisaasConfig{
			BaseURL:       "http://localhost:8190",
			InternalToken: "test-token",
			HTTPTimeout:   "30s",
		},
		VAD: VADConfig{
			SpeechThreshold:  1.5,
			SilenceThreshold: 0.3,
			FrameSizeSamples: 512,
		},
		Opus: OpusConfig{
			Uplink: OpusDirection{
				SampleRate: 16000,
				Channels:   1,
			},
			Downlink: OpusDirection{
				SampleRate: 24000,
				Channels:   1,
			},
		},
		Dialogue: DialogueConfig{
			PerTurnTimeoutSec: 30,
			WindowMemorySize:  20,
		},
		Database: DatabaseConfig{
			DSN:          "postgres://localhost/db",
			MaxOpenConns: 50,
			MaxIdleConns: 10,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for VAD threshold > 1, got nil")
	}
}
