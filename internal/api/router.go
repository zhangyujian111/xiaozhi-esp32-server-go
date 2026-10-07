package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
)

func SetupRouter(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/metrics", authMetrics(cfg.Server.InternalToken), gin.WrapH(promhttp.Handler()))

	return r
}

func SetupAPIRouter(cfg *config.Config, ds store.DeviceStore) *gin.Engine {
	r := SetupRouter(cfg)

	aisaasClient := aisaas.NewClient(cfg.Aisaas.BaseURL, cfg.Aisaas.InternalToken)
	latestFW := "1.1.0"
	publicWSURL := "wss://" + cfg.Server.WebsocketAddr

	otaHandler := NewOTAHandler(ds, aisaasClient, latestFW, publicWSURL, zerolog.Nop())
	r.POST("/api/device/ota", otaHandler.HandleOTA)

	// PR-4：设备轮询"我激活了吗"端点（对齐 xiaozhi-java DeviceController.otaActivate）
	activateHandler := NewOTAActivateHandler(aisaasClient)
	r.GET("/api/device/ota/activate", activateHandler.HandleActivate)

	return r
}

func authMetrics(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearer(c.GetHeader("Authorization"))
		if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

func extractBearer(h string) string {
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

func SetupWebSocketRouter() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
}

func internalJWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		tokenStr := extractBearer(authHeader)
		if tokenStr == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secret), nil
		})
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

func SetupInternalRouter(secret string, ds store.DeviceStore) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	dh := NewDeviceHandler(ds)
	devGroup := r.Group("/api/internal/v1")
	devGroup.Use(internalJWTAuth(secret))
	devGroup.GET("/devices", dh.ListDevices)
	devGroup.GET("/devices/:deviceID", dh.GetDevice)
	devGroup.POST("/devices/:deviceID/bind", dh.BindDevice)
	devGroup.POST("/devices/:deviceID/unbind", dh.UnbindDevice)
	devGroup.DELETE("/devices/:deviceID", dh.DeleteDevice)

	return r
}
