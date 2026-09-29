package config

import (
	"path/filepath"
	"testing"
)

func TestLoad_FullSchema(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "configs", "config.yaml"))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Server.WebsocketAddr != ":8080" {
		t.Errorf("expected websocket_addr :8080, got %s", cfg.Server.WebsocketAddr)
	}
	if cfg.Server.AdminAddr != ":8081" {
		t.Errorf("expected admin_addr :8081, got %s", cfg.Server.AdminAddr)
	}
	if cfg.Server.ReadTimeout != "30s" {
		t.Errorf("expected read_timeout 30s, got %s", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != "30s" {
		t.Errorf("expected write_timeout 30s, got %s", cfg.Server.WriteTimeout)
	}
	if cfg.Server.MaxConns != 200 {
		t.Errorf("expected max_connections 200, got %d", cfg.Server.MaxConns)
	}

	if cfg.Aisaas.BaseURL != "http://ykt-aisaas:8190" {
		t.Errorf("expected base_url http://ykt-aisaas:8190, got %s", cfg.Aisaas.BaseURL)
	}
	if cfg.Aisaas.InternalToken != "${AISAAS_INTERNAL_TOKEN}" {
		t.Errorf("expected internal_token ${AISAAS_INTERNAL_TOKEN}, got %s", cfg.Aisaas.InternalToken)
	}
	if cfg.Aisaas.HTTPTimeout != "30s" {
		t.Errorf("expected http_timeout 30s, got %s", cfg.Aisaas.HTTPTimeout)
	}

	if cfg.VAD.ModelPath != "./models/silero_vad.onnx" {
		t.Errorf("expected model_path ./models/silero_vad.onnx, got %s", cfg.VAD.ModelPath)
	}
	if cfg.VAD.SpeechThreshold != 0.5 {
		t.Errorf("expected speech_threshold 0.5, got %f", cfg.VAD.SpeechThreshold)
	}
	if cfg.VAD.SilenceThreshold != 0.3 {
		t.Errorf("expected silence_threshold 0.3, got %f", cfg.VAD.SilenceThreshold)
	}
	if cfg.VAD.SilenceDurationMs != 800 {
		t.Errorf("expected silence_duration_ms 800, got %d", cfg.VAD.SilenceDurationMs)
	}
	if cfg.VAD.FrameSizeSamples != 512 {
		t.Errorf("expected frame_size_samples 512, got %d", cfg.VAD.FrameSizeSamples)
	}

	if cfg.Opus.Uplink.SampleRate != 16000 {
		t.Errorf("expected uplink sample_rate 16000, got %d", cfg.Opus.Uplink.SampleRate)
	}
	if cfg.Opus.Uplink.Channels != 1 {
		t.Errorf("expected uplink channels 1, got %d", cfg.Opus.Uplink.Channels)
	}
	if cfg.Opus.Uplink.FrameDurationMs != 60 {
		t.Errorf("expected uplink frame_duration_ms 60, got %d", cfg.Opus.Uplink.FrameDurationMs)
	}
	if cfg.Opus.Downlink.SampleRate != 24000 {
		t.Errorf("expected downlink sample_rate 24000, got %d", cfg.Opus.Downlink.SampleRate)
	}
	if cfg.Opus.Downlink.Channels != 1 {
		t.Errorf("expected downlink channels 1, got %d", cfg.Opus.Downlink.Channels)
	}
	if cfg.Opus.Downlink.FrameDurationMs != 60 {
		t.Errorf("expected downlink frame_duration_ms 60, got %d", cfg.Opus.Downlink.FrameDurationMs)
	}

	if cfg.Dialogue.PerTurnTimeoutSec != 30 {
		t.Errorf("expected per_turn_timeout 30, got %d", cfg.Dialogue.PerTurnTimeoutSec)
	}
	if cfg.Dialogue.WindowMemorySize != 20 {
		t.Errorf("expected window_memory_size 20, got %d", cfg.Dialogue.WindowMemorySize)
	}
	if cfg.Dialogue.TTFSAlertThresholdMs != 3000 {
		t.Errorf("expected ttfs_alert_threshold_ms 3000, got %d", cfg.Dialogue.TTFSAlertThresholdMs)
	}

	if cfg.Database.DSN != "${DB_DSN}" {
		t.Errorf("expected dsn ${DB_DSN}, got %s", cfg.Database.DSN)
	}
	if cfg.Database.MaxOpenConns != 50 {
		t.Errorf("expected max_open_conns 50, got %d", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 10 {
		t.Errorf("expected max_idle_conns 10, got %d", cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetime != "30m" {
		t.Errorf("expected conn_max_lifetime 30m, got %s", cfg.Database.ConnMaxLifetime)
	}
	if cfg.Database.MigrationPath != "./migrations" {
		t.Errorf("expected migration_path ./migrations, got %s", cfg.Database.MigrationPath)
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("expected level info, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected format json, got %s", cfg.Logging.Format)
	}
	if cfg.Logging.Output != "stdout" {
		t.Errorf("expected output stdout, got %s", cfg.Logging.Output)
	}
}
