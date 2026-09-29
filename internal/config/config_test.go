package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
}

func TestLoad_EnvSubstitution(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	yaml := []byte("server:\n  websocket_addr: \":8080\"\n  admin_addr: \":8081\"\n  read_timeout: \"30s\"\n  write_timeout: \"30s\"\n  max_connections: 100\n  internal_token: \"${TEST_INTERNAL_TOKEN}\"\naisaas:\n  base_url: \"http://x\"\n  internal_token: \"${TEST_AISAAS_TOKEN}\"\n  http_timeout: \"30s\"\nvad:\n  model_path: \"./x\"\n  speech_threshold: 0.5\n  silence_threshold: 0.3\n  silence_duration_ms: 800\n  frame_size_samples: 512\nopus:\n  uplink:\n    sample_rate: 16000\n    channels: 1\n    frame_duration_ms: 60\n  downlink:\n    sample_rate: 24000\n    channels: 1\n    frame_duration_ms: 60\ndialogue:\n  per_turn_timeout: 30\n  window_memory_size: 20\n  ttfs_alert_threshold_ms: 3000\ndatabase:\n  dsn: \"${TEST_DB_DSN}\"\n  max_open_conns: 50\n  max_idle_conns: 10\n  conn_max_lifetime: \"30m\"\n  migration_path: \"./migrations\"\nlogging:\n  level: info\n  format: json\n  output: stdout\n")
	if err := os.WriteFile(cfgPath, yaml, 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	t.Setenv("TEST_INTERNAL_TOKEN", "real-server-tok")
	t.Setenv("TEST_AISAAS_TOKEN", "real-aisaas-tok")
	t.Setenv("TEST_DB_DSN", "postgres://real/db")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Server.InternalToken != "real-server-tok" {
		t.Errorf("expected internal_token to be substituted, got %q", cfg.Server.InternalToken)
	}
	if cfg.Aisaas.InternalToken != "real-aisaas-tok" {
		t.Errorf("expected aisaas.internal_token to be substituted, got %q", cfg.Aisaas.InternalToken)
	}
	if cfg.Database.DSN != "postgres://real/db" {
		t.Errorf("expected database.dsn to be substituted, got %q", cfg.Database.DSN)
	}
}
