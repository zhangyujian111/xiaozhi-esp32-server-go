package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/api"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/obs"
)

func main() {
	cfg, err := config.Load("./configs/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	logger := obs.InitLogger(cfg.Logging.Level, cfg.Logging.Format)
	logger.Info().Msg("xiaozhi-esp32-server-go started")

	adminSrv := &http.Server{
		Addr:         cfg.Server.AdminAddr,
		Handler:      api.SetupRouter(),
		ReadTimeout:  parseDuration(cfg.Server.ReadTimeout),
		WriteTimeout: parseDuration(cfg.Server.WriteTimeout),
	}

	wsSrv := &http.Server{
		Addr:         cfg.Server.WebsocketAddr,
		Handler:      api.SetupWebSocketRouter(),
		ReadTimeout:  parseDuration(cfg.Server.ReadTimeout),
		WriteTimeout: parseDuration(cfg.Server.WriteTimeout),
	}

	go func() {
		if err := adminSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("admin server failed")
		}
	}()
	go func() {
		if err := wsSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("websocket server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := adminSrv.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("admin server shutdown error")
	}
	if err := wsSrv.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("ws server shutdown error")
	}
}

func parseDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	if d == 0 {
		d = 30 * time.Second
	}
	return d
}
