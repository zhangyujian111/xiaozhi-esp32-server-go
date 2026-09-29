package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/app"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
)

func main() {
	flag.Parse()

	cfg, err := config.Load("./configs/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	appInstance, err := app.NewApp(cfg)
	if err != nil {
		log.Fatalf("failed to create app: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		cancel()
	}()

	if err := appInstance.Run(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
