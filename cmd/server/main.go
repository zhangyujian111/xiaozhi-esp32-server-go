package main

import (
	"log"

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
}
