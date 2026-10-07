package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
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
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/vad"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/dialogue"
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
	deviceStorePath := filepath.Join("data", "devices.json")
	deviceStore, err := store.NewFileDeviceStore(deviceStorePath)
	if err != nil {
		return nil, fmt.Errorf("init device store at %s: %w", deviceStorePath, err)
	}
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
	wsHandler.SetDeviceClient(aisaasClient)
	wsHandler.SetOpusDecoder(opusDecoder)
	wsHandler.SetDeviceStore(deviceStore)

	// Server-side VAD (Silero). When the model file is present and the binary
	// was built with `-tags silero`, every opus frame is decoded and pushed
	// through the pipeline. SpeechStart aborts any in-flight TTS; SpeechEnd
	// triggers the orchestrator (drains the audio buffer + runs STT→Chat→TTS).
	// Without the silero build tag or with a missing model, the handler still
	// buffers audio as before; the listen-stop path is the fallback.
	var audioPipeline *ws.AudioPipeline
	if cfg.VAD.ModelPath != "" {
		silero, vErr := vad.NewSileroVAD(cfg.VAD.ModelPath)
		if vErr != nil {
			logger.Warn().Err(vErr).Str("model_path", cfg.VAD.ModelPath).Msg("silero VAD unavailable; falling back to listen-stop only")
		} else {
			audioPipeline = ws.NewAudioPipelineWithLog(
				sessionManager,
				silero,
				cfg.VAD.SpeechThreshold,
				cfg.VAD.SilenceThreshold,
				cfg.VAD.SilenceDurationMs,
				logger,
			)
			wsHandler.SetPipeline(audioPipeline)
			logger.Info().Str("model_path", cfg.VAD.ModelPath).Msg("silero VAD pipeline wired")
		}
	}

	// Dialogue orchestrator：audio -> STT -> Chat (persona) -> TTS -> 回写
	sentenceSplitter := dialogue.NewSentenceSplitter()
	orchestrator := dialogue.NewOrchestrator(aisaasClient, sentenceSplitter, memory, opusEncoder, logger).
		WithFramePacer(dialogue.NewFramePacer(60 * time.Millisecond))
	wsHandler.SetOrchestrator(orchestrator)

	// Streaming dialogue orchestrator (aisaas /dialogue/stream SSE edge-push).
	// When cfg.Dialogue.StreamingEnabled is true, ws.Handler prefers this path
	// for lower latency (LLM stream + TTS stream); otherwise it falls back to
	// the legacy Orchestrator above.
	streamClientFactory := func(apiKey string) *aisaas.StreamClient {
		sc := aisaas.NewStreamClient(cfg.Aisaas.BaseURL, cfg.Aisaas.InternalToken)
		sc.APIKey = apiKey // per-session; 由 invokeOrchestrator 在调 Run 前注入
		return sc
	}
	streamOrchestrator := dialogue.NewStreamOrchestrator(streamClientFactory, opusEncoder, logger).
		WithFramePacer(dialogue.NewFramePacer(60 * time.Millisecond))
	wsHandler.SetStreamOrchestrator(streamOrchestrator, cfg.Dialogue.StreamingEnabled)

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
	publicWSURL := cfg.Server.PublicWSURL
	if publicWSURL == "" {
		publicWSURL = "wss://" + cfg.Server.WebsocketAddr
		logger.Warn().Msgf("server.public_ws_url not set, fallback to %q (may be invalid for hardware)", publicWSURL)
	}
	otaHandler := api.NewOTAHandler(deviceStore, aisaasClient, latestFW, publicWSURL, logger)
	adminMux.POST("/api/device/ota", otaHandler.HandleOTA)

	// PR-4：设备轮询"我激活了吗"端点（对齐 xiaozhi-java DeviceController.otaActivate）
	activateHandler := api.NewOTAActivateHandler(aisaasClient)
	adminMux.GET("/api/device/ota/activate", activateHandler.HandleActivate)

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

	publicURL, _ := url.Parse(a.cfg.Server.PublicWSURL)
	publicHost := ""
	if publicURL != nil {
		publicHost = publicURL.Hostname()
	}
	if publicHost == "" {
		publicHost = "127.0.0.1"
	}

	g.Go(func() error {
		a.logger.Info().
			Str("addr", a.AdminSrv.Addr).
			Str("public_http_url", "http://"+publicHost+a.AdminSrv.Addr).
			Str("ota_url", "http://"+publicHost+a.AdminSrv.Addr+"/api/device/ota").
			Str("ota_activate_url", "http://"+publicHost+a.AdminSrv.Addr+"/api/device/ota/activate").
			Msg("admin server listening")
		if err := a.AdminSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	g.Go(func() error {
		a.logger.Info().
			Str("addr", a.WsSrv.Addr).
			Str("public_ws_url", "ws://"+publicHost+a.WsSrv.Addr+"/ws").
			Msg("websocket server listening")
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
