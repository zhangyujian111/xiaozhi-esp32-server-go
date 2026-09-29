package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	api "github.com/xiaozhi/xiaozhi-esp32-server-go/internal/api"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/opus"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/event"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/obs"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/ws"
	"golang.org/x/sync/errgroup"
)

type App struct {
	cfg          *config.Config
	logger       zerolog.Logger
	aisaasClient *aisaas.Client
	memory       store.Memory
	deviceStore  store.DeviceStore
	eventBus     *event.EventBus
	opusDecoder  *opus.Decoder
	opusEncoder  *opus.Encoder
	wsHandler    *ws.Handler
	AdminSrv     *http.Server
	WsSrv        *http.Server
	shutdownOnce sync.Once
}

func NewApp(cfg *config.Config) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	logger := obs.InitLogger(cfg.Logging.Level, cfg.Logging.Format)

	memory := store.NewInMemoryMemory()
	deviceStore := store.NewInMemoryDeviceStore()
	eventBus := event.NewEventBus(100)

	aisaasClient := aisaas.NewClient(cfg.Aisaas.BaseURL, cfg.Aisaas.InternalToken)

	opusDecoder, err := opus.NewDecoder(cfg.Opus.Uplink.SampleRate, cfg.Opus.Uplink.Channels)
	if err != nil {
		return nil, err
	}
	opusEncoder, err := opus.NewEncoder(cfg.Opus.Downlink.SampleRate, cfg.Opus.Downlink.Channels)
	if err != nil {
		return nil, err
	}

	sessionManager := ws.NewSessionManager()
	wsHandler := ws.NewHandler(sessionManager, aisaasClient, logger)
	wsHandler.SetEventBus(eventBus)

	gin.SetMode(gin.ReleaseMode)
	adminMux := gin.New()
	adminMux.Use(gin.Recovery())

	adminMux.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	adminMux.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	adminMux.GET("/metrics", metricsAuth(cfg.Server.InternalToken), gin.WrapH(promhttp.Handler()))

	latestFW := "1.1.0"
	publicWSURL := "wss://" + cfg.Server.WebsocketAddr
	otaHandler := api.NewOTAHandler(deviceStore, aisaasClient, latestFW, publicWSURL)
	adminMux.POST("/api/device/ota", otaHandler.HandleOTA)

	dh := api.NewDeviceHandler(deviceStore)
	internal := adminMux.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(cfg.Server.InternalToken))
	internal.GET("/devices", dh.ListDevices)
	internal.GET("/devices/:deviceID", dh.GetDevice)
	internal.POST("/devices/:deviceID/bind", dh.BindDevice)
	internal.POST("/devices/:deviceID/unbind", dh.UnbindDevice)
	internal.DELETE("/devices/:deviceID", dh.DeleteDevice)

	adminSrv := &http.Server{
		Addr:         cfg.Server.AdminAddr,
		Handler:      adminMux,
		ReadTimeout:  parseDuration(cfg.Server.ReadTimeout),
		WriteTimeout: parseDuration(cfg.Server.WriteTimeout),
	}

	wsMux := http.NewServeMux()
	wsMux.Handle("/ws", http.HandlerFunc(wsHandler.HandleUpgrade))
	wsSrv := &http.Server{
		Addr:         cfg.Server.WebsocketAddr,
		Handler:      wsMux,
		ReadTimeout:  parseDuration(cfg.Server.ReadTimeout),
		WriteTimeout: parseDuration(cfg.Server.WriteTimeout),
	}

	return &App{
		cfg:          cfg,
		logger:       logger,
		aisaasClient: aisaasClient,
		memory:       memory,
		deviceStore:  deviceStore,
		eventBus:     eventBus,
		opusDecoder:  opusDecoder,
		opusEncoder:  opusEncoder,
		wsHandler:    wsHandler,
		AdminSrv:     adminSrv,
		WsSrv:        wsSrv,
	}, nil
}

func (a *App) AdminHandler() http.Handler {
	return a.AdminSrv.Handler
}

func (a *App) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := a.AdminSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	g.Go(func() error {
		if err := a.WsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		a.logger.Info().Str("signal", sig.String()).Msg("received shutdown signal")
	case <-ctx.Done():
		a.logger.Error().Err(ctx.Err()).Msg("context cancelled")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_ = a.AdminSrv.Shutdown(shutdownCtx)
	_ = a.WsSrv.Shutdown(shutdownCtx)

	return g.Wait()
}

func (a *App) Shutdown(ctx context.Context) error {
	var err error
	a.shutdownOnce.Do(func() {
		shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		_ = a.AdminSrv.Shutdown(shutdownCtx)
		_ = a.WsSrv.Shutdown(shutdownCtx)
	})
	return err
}

func parseDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	if d == 0 {
		d = 30 * time.Second
	}
	return d
}

func metricsAuth(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearer(c.GetHeader("Authorization"))
		if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

func internalJWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		tokenStr := extractBearer(authHeader)
		if tokenStr == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		token, err := jwtParse(tokenStr, secret)
		if err != nil || !token.Valid {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if role, ok := claims["role"].(string); !ok || role != "admin" {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()
	}
}

func jwtParse(tokenStr, secret string) (*jwt.Token, error) {
	return jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})
}

func extractBearer(h string) string {
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
