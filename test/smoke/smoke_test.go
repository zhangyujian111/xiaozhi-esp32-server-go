package smoke

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/app"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
)

var testConfig = &config.Config{
	Server: config.ServerConfig{
		WebsocketAddr: "127.0.0.1:0",
		AdminAddr:     "localhost:0",
		ReadTimeout:   "30s",
		WriteTimeout:  "30s",
		MaxConns:      100,
		InternalToken: "test-internal-token",
	},
	Database: config.DatabaseConfig{
		DSN:             "test:test@tcp(localhost:3306)/test?parseTime=true",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: "1h",
	},
	Logging: config.LoggingConfig{
		Level:  "debug",
		Format: "console",
	},
	Aisaas: config.AisaasConfig{
		BaseURL:       "http://localhost:9999",
		InternalToken: "test-internal-token",
		HTTPTimeout:   "10s",
	},
	VAD: config.VADConfig{
		SpeechThreshold:   0.5,
		SilenceThreshold:  0.5,
		SilenceDurationMs: 500,
		FrameSizeSamples:  480,
	},
	Opus: config.OpusConfig{
		Uplink:   config.OpusDirection{SampleRate: 16000, Channels: 1, FrameDurationMs: 60},
		Downlink: config.OpusDirection{SampleRate: 24000, Channels: 1, FrameDurationMs: 60},
	},
	Dialogue: config.DialogueConfig{
		PerTurnTimeoutSec:    30,
		WindowMemorySize:     10,
		TTFSAlertThresholdMs: 1000,
	},
}

type smokeEnv struct {
	appInstance *app.App
	adminServer *http.Server
	wsServer    *http.Server
}

func startSmokeApp(t *testing.T) (*smokeEnv, string, string) {
	testCfg := *testConfig

	mockAisaas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/api/v1/devices/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"device_id":"` + r.URL.Path[len("/internal/v1/devices/"):] + `","name":"test-device","activated":true}`))
			return
		}
		http.NotFound(w, r)
	}))
	testCfg.Aisaas.BaseURL = mockAisaas.URL

	t.Cleanup(func() { mockAisaas.Close() })

	appInstance, err := app.NewApp(&testCfg)
	require.NoError(t, err)

	adminListener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	adminAddr := adminListener.Addr().String()

	wsListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	wsAddr := wsListener.Addr().String()

	adminDone := make(chan struct{})
	go func() {
		defer close(adminDone)
		appInstance.AdminSrv.Serve(adminListener)
	}()

	wsDone := make(chan struct{})
	go func() {
		defer close(wsDone)
		appInstance.WsSrv.Serve(wsListener)
	}()

	env := &smokeEnv{
		appInstance: appInstance,
		adminServer: appInstance.AdminSrv,
		wsServer:    appInstance.WsSrv,
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		env.adminServer.Shutdown(ctx)
		env.wsServer.Shutdown(ctx)
		<-adminDone
		<-wsDone
	})

	return env, adminAddr, wsAddr
}

func generateAdminJWT(t *testing.T, secret string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"role": "admin",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func TestSmoke_Healthz(t *testing.T) {
	_, adminAddr, _ := startSmokeApp(t)

	resp, err := http.Get("http://" + adminAddr + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body["status"])
}

func TestSmoke_Readyz(t *testing.T) {
	_, adminAddr, _ := startSmokeApp(t)

	resp, err := http.Get("http://" + adminAddr + "/readyz")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body["status"])
}

func TestSmoke_Metrics_Unauthorized(t *testing.T) {
	_, adminAddr, _ := startSmokeApp(t)

	resp, err := http.Get("http://" + adminAddr + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestSmoke_Metrics_Authorized(t *testing.T) {
	_, adminAddr, _ := startSmokeApp(t)

	req, err := http.NewRequest("GET", "http://"+adminAddr+"/metrics", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-internal-token")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/plain")
}

func TestSmoke_OTA_MissingHeaders(t *testing.T) {
	_, adminAddr, _ := startSmokeApp(t)

	req, err := http.NewRequest("POST", "http://"+adminAddr+"/api/device/ota", strings.NewReader(`{"current_firmware_version":"1.0.0"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSmoke_OTA_Success(t *testing.T) {
	_, adminAddr, _ := startSmokeApp(t)

	req, err := http.NewRequest("POST", "http://"+adminAddr+"/api/device/ota", strings.NewReader(`{"current_firmware_version":"1.0.0"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Device-Id", "test-device-001")
	req.Header.Set("Client-Id", "test-client")
	req.Header.Set("Activation-Version", "1")
	req.Header.Set("User-Agent", "ESP32/1.0.0")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Contains(t, body, "websocket")
	assert.Contains(t, body, "firmware")
}

func TestSmoke_InternalDevices_NoJWT(t *testing.T) {
	_, adminAddr, _ := startSmokeApp(t)

	resp, err := http.Get("http://" + adminAddr + "/api/internal/v1/devices")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestSmoke_InternalDevices_WithJWT(t *testing.T) {
	_, adminAddr, _ := startSmokeApp(t)

	jwt := generateAdminJWT(t, "test-internal-token")
	req, err := http.NewRequest("GET", "http://"+adminAddr+"/api/internal/v1/devices", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Devices []interface{} `json:"devices"`
		Total   int           `json:"total"`
	}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.NotNil(t, body.Devices)
}

func TestSmoke_WebSocket_Hello(t *testing.T) {
	_, _, wsAddr := startSmokeApp(t)

	u := url.URL{Scheme: "ws", Host: wsAddr, Path: "/ws"}
	h := http.Header{}
	h.Set("Device-Id", "test-device-001")
	h.Set("Authorization", "Bearer test-token")
	h.Set("Protocol-Version", "1")

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), h)
	require.NoError(t, err)
	defer conn.Close()

	hello := map[string]interface{}{
		"type":      "hello",
		"version":   1,
		"transport": "websocket",
		"features":  map[string]bool{"MCP": true, "AEC": true},
		"audio_params": map[string]interface{}{
			"format":         "opus",
			"sample_rate":    16000,
			"channels":       1,
			"frame_duration": 60,
		},
	}
	err = conn.WriteJSON(hello)
	require.NoError(t, err)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp map[string]interface{}
	err = conn.ReadJSON(&resp)
	require.NoError(t, err)
	assert.Equal(t, "hello", resp["type"])
	assert.NotEmpty(t, resp["session_id"])
}

func TestSmoke_WebSocket_ListenStart(t *testing.T) {
	_, _, wsAddr := startSmokeApp(t)

	u := url.URL{Scheme: "ws", Host: wsAddr, Path: "/ws"}
	h := http.Header{}
	h.Set("Device-Id", "test-device-001")
	h.Set("Authorization", "Bearer test-token")
	h.Set("Protocol-Version", "1")

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), h)
	require.NoError(t, err)
	defer conn.Close()

	hello := map[string]interface{}{
		"type":      "hello",
		"version":   1,
		"transport": "websocket",
		"features":  map[string]bool{"MCP": true, "AEC": true},
		"audio_params": map[string]interface{}{
			"format":         "opus",
			"sample_rate":    16000,
			"channels":       1,
			"frame_duration": 60,
		},
	}
	err = conn.WriteJSON(hello)
	require.NoError(t, err)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp map[string]interface{}
	err = conn.ReadJSON(&resp)
	require.NoError(t, err)
	assert.Equal(t, "hello", resp["type"])

	listenMsg := map[string]interface{}{
		"type":  "listen",
		"state": "start",
	}
	err = conn.WriteJSON(listenMsg)
	require.NoError(t, err)
}
