package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
)

func TestSetupRouter_Healthz(t *testing.T) {
	cfg := &config.Config{}
	r := SetupRouter(cfg)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestSetupRouter_Readyz(t *testing.T) {
	cfg := &config.Config{}
	r := SetupRouter(cfg)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestSetupRouter_Metrics_Unauthorized(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{InternalToken: "secret"}}
	r := SetupRouter(cfg)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSetupRouter_Metrics_Authorized(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{InternalToken: "secret"}}
	r := SetupRouter(cfg)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestSetupWebSocketRouter(t *testing.T) {
	r := SetupWebSocketRouter()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestSetupInternalRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ds := store.NewInMemoryDeviceStore()
	r := SetupInternalRouter("jwt-secret", ds)
	req := httptest.NewRequest(http.MethodGet, "/api/internal/v1/devices", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("jwt-secret"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestSetupInternalRouter_Unauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ds := store.NewInMemoryDeviceStore()
	r := SetupInternalRouter("jwt-secret", ds)
	req := httptest.NewRequest(http.MethodGet, "/api/internal/v1/devices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSetupInternalProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, err := SetupInternalProxy("http://localhost:9999", "jwt-secret")
	require.NoError(t, err)
	require.NotNil(t, r)
}

func TestSetupInternalProxy_BadURL(t *testing.T) {
	r, err := SetupInternalProxy("://invalid", "jwt-secret")
	require.Error(t, err)
	require.Nil(t, r)
}

func TestInternalJWTAuth_ValidAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(internalJWTAuth("secret"))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"role": "admin"})
	tokenStr, _ := token.SignedString([]byte("secret"))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestInternalJWTAuth_ExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(internalJWTAuth("secret"))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"role": "admin",
		"exp":  float64(0),
	})
	tokenStr, _ := token.SignedString([]byte("secret"))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestInternalJWTAuth_WrongSigningMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(internalJWTAuth("secret"))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	token := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.MapClaims{"role": "admin"})
	tokenStr, _ := token.SignedString([]byte("secret"))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestDeviceCRUD_GetDevice_StoreError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ms := &mockDeviceStore{getErr: context.DeadlineExceeded}
	dh := NewDeviceHandler(ms)
	secret := "test-secret"
	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.GET("/devices/:deviceID", dh.GetDevice)

	req := httptest.NewRequest(http.MethodGet, "/api/internal/v1/devices/dev1", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT(secret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestDeviceCRUD_ListDevices_StoreError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ms := &mockDeviceStore{listErr: context.DeadlineExceeded}
	dh := NewDeviceHandler(ms)
	secret := "test-secret"
	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.GET("/devices", dh.ListDevices)

	req := httptest.NewRequest(http.MethodGet, "/api/internal/v1/devices", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT(secret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestDeviceCRUD_BindDevice_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ms := &mockDeviceStore{bindErr: store.ErrDeviceNotFound}
	dh := NewDeviceHandler(ms)
	secret := "test-secret"
	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.POST("/devices/:deviceID/bind", dh.BindDevice)

	req := httptest.NewRequest(http.MethodPost, "/api/internal/v1/devices/dev1/bind", bytes.NewReader([]byte(`{"user_id":123}`)))
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT(secret))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeviceCRUD_UnbindDevice_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ms := &mockDeviceStore{unbindErr: store.ErrDeviceNotFound}
	dh := NewDeviceHandler(ms)
	secret := "test-secret"
	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.POST("/devices/:deviceID/unbind", dh.UnbindDevice)

	req := httptest.NewRequest(http.MethodPost, "/api/internal/v1/devices/dev1/unbind", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT(secret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeviceCRUD_DeleteDevice_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ms := &mockDeviceStore{getErr: store.ErrDeviceNotFound}
	dh := NewDeviceHandler(ms)
	secret := "test-secret"
	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.DELETE("/devices/:deviceID", dh.DeleteDevice)

	req := httptest.NewRequest(http.MethodDelete, "/api/internal/v1/devices/dev1", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT(secret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestOTAHandler_AisaasGetDeviceError_500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ms := &mockOTAStore{devices: make(map[string]*store.Device)}
	mc := &mockOTAClient{getDeviceErr: context.DeadlineExceeded}
	h := NewOTAHandler(ms, mc, "1.1.0", "wss://example.com/ws", zerolog.Nop())
	r.POST("/api/device/ota", h.HandleOTA)

	req := httptest.NewRequest(http.MethodPost, "/api/device/ota", bytes.NewReader([]byte(`{"current_firmware_version":"1.0.0"}`)))
	req.Header.Set("Activation-Version", "1")
	req.Header.Set("Device-Id", "AA:BB:CC:DD:EE:FF")
	req.Header.Set("Client-Id", "clientA")
	req.Header.Set("User-Agent", "ESP32/1.0.0")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestOTAHandler_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ms := &mockOTAStore{devices: make(map[string]*store.Device)}
	mc := &mockOTAClient{getDeviceResp: &aisaas.DeviceInfo{DeviceID: "AA:BB:CC:DD:EE:FF", TenantID: 1, BindCode: "x"}}
	h := NewOTAHandler(ms, mc, "1.1.0", "wss://example.com/ws", zerolog.Nop())
	r.POST("/api/device/ota", h.HandleOTA)

	req := httptest.NewRequest(http.MethodPost, "/api/device/ota", nil)
	req.Header.Set("Activation-Version", "1")
	req.Header.Set("Device-Id", "AA:BB:CC:DD:EE:FF")
	req.Header.Set("Client-Id", "clientA")
	req.Header.Set("User-Agent", "ESP32/1.0.0")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
