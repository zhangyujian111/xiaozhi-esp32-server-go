package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/api"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/obs"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/server"
)

func main() {
	cfg, err := config.Load("./configs/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config validation failed: %v", err)
	}
	logger := obs.InitLogger(cfg.Logging.Level, cfg.Logging.Format)
	logger.Info().Msg("xiaozhi-esp32-server-go started")

	adminSrv := &http.Server{
		Addr:         cfg.Server.AdminAddr,
		Handler:      api.SetupRouter(cfg),
		ReadTimeout:  parseDuration(cfg.Server.ReadTimeout),
		WriteTimeout: parseDuration(cfg.Server.WriteTimeout),
	}

	wsSrv := &http.Server{
		Addr:         cfg.Server.WebsocketAddr,
		Handler:      api.SetupWebSocketRouter(),
		ReadTimeout:  parseDuration(cfg.Server.ReadTimeout),
		WriteTimeout: parseDuration(cfg.Server.WriteTimeout),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		cancel()
	}()

	if err := server.RunServers(ctx, adminSrv, wsSrv, &logger); err != nil {
		logger.Error().Err(err).Msg("server error")
	}
}

func parseDuration(s string) (d time.Duration) {
	d, _ = time.ParseDuration(s)
	if d == 0 {
		d = 30 * time.Second
	}
	return d
}
