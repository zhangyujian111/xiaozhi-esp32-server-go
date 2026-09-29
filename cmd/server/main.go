package main

import (
	"log"

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

	r := api.SetupRouter()
	if err := r.Run(":8081"); err != nil {
		logger.Fatal().Err(err).Msg("failed to start server")
	}
}
