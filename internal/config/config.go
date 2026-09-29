package config

import (
	"fmt"

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
	ReadTimeout   string `mapstructure:"read_timeout"`
	WriteTimeout  string `mapstructure:"write_timeout"`
	MaxConns      int    `mapstructure:"max_connections"`
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
	viper.SetConfigFile(path)

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}
