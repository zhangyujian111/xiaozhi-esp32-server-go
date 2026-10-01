package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Aisaas   AisaasConfig   `mapstructure:"aisaas"`
	VAD      VADConfig      `mapstructure:"vad"`
	Opus     OpusConfig     `mapstructure:"opus"`
	Dialogue DialogueConfig `mapstructure:"dialogue"`
	Database DatabaseConfig `mapstructure:"database"`
	Logging  LoggingConfig  `mapstructure:"logging"`
}

type ServerConfig struct {
	WebsocketAddr string `mapstructure:"websocket_addr"`
	AdminAddr     string `mapstructure:"admin_addr"`
	PublicWSURL   string `mapstructure:"public_ws_url"` // OTA 返回给硬件的对外地址（覆盖自动拼接）
	ReadTimeout   string `mapstructure:"read_timeout"`
	WriteTimeout  string `mapstructure:"write_timeout"`
	MaxConns      int    `mapstructure:"max_connections"`
	InternalToken string `mapstructure:"internal_token"`
}

type AisaasConfig struct {
	BaseURL       string `mapstructure:"base_url"`
	InternalToken string `mapstructure:"internal_token"`
	HTTPTimeout   string `mapstructure:"http_timeout"`
}

type VADConfig struct {
	ModelPath         string  `mapstructure:"model_path"`
	SpeechThreshold   float32 `mapstructure:"speech_threshold"`
	SilenceThreshold  float32 `mapstructure:"silence_threshold"`
	SilenceDurationMs int     `mapstructure:"silence_duration_ms"`
	FrameSizeSamples  int     `mapstructure:"frame_size_samples"`
}

type OpusDirection struct {
	SampleRate      int `mapstructure:"sample_rate"`
	Channels        int `mapstructure:"channels"`
	FrameDurationMs int `mapstructure:"frame_duration_ms"`
}

type OpusConfig struct {
	Uplink   OpusDirection `mapstructure:"uplink"`
	Downlink OpusDirection `mapstructure:"downlink"`
}

type DialogueConfig struct {
	PerTurnTimeoutSec    int `mapstructure:"per_turn_timeout"`
	WindowMemorySize     int `mapstructure:"window_memory_size"`
	TTFSAlertThresholdMs int `mapstructure:"ttfs_alert_threshold_ms"`
}

type DatabaseConfig struct {
	DSN             string `mapstructure:"dsn"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime string `mapstructure:"conn_max_lifetime"`
	MigrationPath   string `mapstructure:"migration_path"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	expanded := []byte(os.ExpandEnv(string(raw)))

	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(bytes.NewReader(expanded)); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	var errs []string

	if c.Server.WebsocketAddr == "" {
		errs = append(errs, "server.websocket_addr is required")
	}
	if c.Server.AdminAddr == "" {
		errs = append(errs, "server.admin_addr is required")
	}
	if c.Server.WebsocketAddr != "" && c.Server.AdminAddr != "" &&
		c.Server.WebsocketAddr == c.Server.AdminAddr {
		errs = append(errs, fmt.Sprintf("server.websocket_addr (%s) and server.admin_addr (%s) must differ", c.Server.WebsocketAddr, c.Server.AdminAddr))
	}
	if c.Server.MaxConns <= 0 {
		errs = append(errs, fmt.Sprintf("server.max_connections must be > 0, got %d", c.Server.MaxConns))
	}
	if c.Server.ReadTimeout == "" || c.Server.WriteTimeout == "" {
		errs = append(errs, "server.read_timeout and server.write_timeout are required")
	}

	if c.Aisaas.BaseURL == "" {
		errs = append(errs, "aisaas.base_url is required")
	}
	if c.Aisaas.InternalToken == "" {
		errs = append(errs, "aisaas.internal_token is required")
	}
	if c.Aisaas.HTTPTimeout == "" {
		errs = append(errs, "aisaas.http_timeout is required")
	}

	if c.VAD.SpeechThreshold < 0 || c.VAD.SpeechThreshold > 1 {
		errs = append(errs, fmt.Sprintf("vad.speech_threshold must be in [0,1], got %f", c.VAD.SpeechThreshold))
	}
	if c.VAD.SilenceThreshold < 0 || c.VAD.SilenceThreshold > 1 {
		errs = append(errs, fmt.Sprintf("vad.silence_threshold must be in [0,1], got %f", c.VAD.SilenceThreshold))
	}
	if c.VAD.FrameSizeSamples <= 0 {
		errs = append(errs, fmt.Sprintf("vad.frame_size_samples must be > 0, got %d", c.VAD.FrameSizeSamples))
	}

	if c.Opus.Uplink.SampleRate <= 0 || c.Opus.Downlink.SampleRate <= 0 {
		errs = append(errs, "opus uplink/downlink sample_rate must be > 0")
	}
	if c.Opus.Uplink.Channels <= 0 || c.Opus.Downlink.Channels <= 0 {
		errs = append(errs, "opus uplink/downlink channels must be > 0")
	}

	if c.Dialogue.WindowMemorySize <= 0 {
		errs = append(errs, "dialogue.window_memory_size must be > 0")
	}
	if c.Dialogue.PerTurnTimeoutSec <= 0 {
		errs = append(errs, "dialogue.per_turn_timeout must be > 0")
	}

	if c.Database.MaxOpenConns <= 0 {
		errs = append(errs, "database.max_open_conns must be > 0")
	}
	if c.Database.MaxIdleConns < 0 || c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		errs = append(errs, fmt.Sprintf("database.max_idle_conns (%d) must be in [0, max_open_conns (%d)]", c.Database.MaxIdleConns, c.Database.MaxOpenConns))
	}
	if c.Database.DSN == "" {
		errs = append(errs, "database.dsn is required")
	}

	if c.Logging.Level == "" {
		errs = append(errs, "logging.level is required")
	}
	if c.Logging.Format != "json" && c.Logging.Format != "console" {
		errs = append(errs, fmt.Sprintf("logging.format must be 'json' or 'console', got %q", c.Logging.Format))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
