package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func TestRunServers_DuplicatePort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve port: %v", err)
	}
	defer ln.Close()
	reservedPort := ln.Addr().String()

	gin.SetMode(gin.TestMode)
	logger := zerolog.Nop()

	adminSrv := &http.Server{
		Addr:    reservedPort,
		Handler: gin.New(),
	}
	wsSrv := &http.Server{
		Addr:    reservedPort,
		Handler: gin.New(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = RunServers(ctx, adminSrv, wsSrv, &logger)

	if err == nil {
		t.Fatal("expected error when both servers bind to same port, got nil")
	}
}

func TestRunServers_OneServerFails(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve port: %v", err)
	}
	defer ln.Close()
	reservedPort := ln.Addr().String()

	gin.SetMode(gin.TestMode)
	logger := zerolog.Nop()

	adminSrv := &http.Server{
		Addr:    reservedPort,
		Handler: gin.New(),
	}
	wsSrv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: gin.New(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = RunServers(ctx, adminSrv, wsSrv, &logger)

	if err == nil {
		t.Fatal("expected error when admin server fails to start, got nil")
	}
}

func TestRunServers_GracefulShutdown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zerolog.Nop()

	adminSrv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: gin.New(),
	}
	wsSrv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: gin.New(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- RunServers(ctx, adminSrv, wsSrv, &logger)
	}()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected context deadline exceeded or nil, got: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("test timed out - RunServers did not return")
	}
}
