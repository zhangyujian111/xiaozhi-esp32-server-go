//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/api"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/event"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/obs"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/ws"
)

var testLogger = obs.InitLogger("debug", "console")

type testEnv struct {
	server     *httptest.Server
	config     *config.Config
	deviceID   string
	token      string
	sessionID  string
	sessionMux sync.Mutex
}

func (te *testEnv) SetSessionID(sid string) {
	te.sessionMux.Lock()
	te.sessionID = sid
	te.sessionMux.Unlock()
}

func (te *testEnv) GetSessionID() string {
	te.sessionMux.Lock()
	defer te.sessionMux.Unlock()
	return te.sessionID
}

func (te *testEnv) WSURL() string {
	return strings.Replace(te.server.URL, "http://", "ws://", 1) + "/ws"
}

func newTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			AdminAddr:     "localhost:0",
			WebsocketAddr: "localhost:0",
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
}

func StartTestServer(t *testing.T) (*testEnv, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	cfg := newTestConfig()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}

	ds := store.NewMySQLDeviceStore(db)

	sm := ws.NewSessionManager()
	wsh := ws.NewHandler(sm, &mockAuth{}, testLogger)
	wsh.SetEventBus(event.NewEventBus(100))

	apiHandler := api.SetupAPIRouter(cfg, ds)

	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/ws", http.HandlerFunc(wsh.HandleUpgrade))

	server := httptest.NewServer(mux)
	t.Cleanup(func() { server.Close(); db.Close() })

	te := &testEnv{
		server:   server,
		config:   cfg,
		deviceID: randomDeviceID(),
		token:    "dev-token",
	}

	return te, mock
}

type mockAuth struct{}

func (m *mockAuth) VerifyDeviceToken(ctx context.Context, deviceID, token string) error {
	return nil
}

func randomDeviceID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "test-" + hex.EncodeToString(b)
}

func OpenDeviceWS(t *testing.T, te *testEnv) *websocket.Conn {
	u, err := url.Parse(te.WSURL())
	if err != nil {
		t.Fatalf("parse ws url: %v", err)
	}
	h := http.Header{}
	h.Set("Device-Id", te.deviceID)
	h.Set("Authorization", "Bearer "+te.token)
	h.Set("Protocol-Version", "1")

	c, _, err := websocket.DefaultDialer.Dial(u.String(), h)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

func DoHello(t *testing.T, c *websocket.Conn, te *testEnv) {
	hello := protocol.HelloMessage{
		Type:      protocol.Hello,
		Version:   1,
		Transport: "websocket",
		Features:  &protocol.HelloFeatures{MCP: true, AEC: true},
		AudioParams: &protocol.AudioParams{
			Format:        "opus",
			SampleRate:    16000,
			Channels:      1,
			FrameDuration: 60,
		},
	}
	if err := c.WriteJSON(hello); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	var resp protocol.HelloMessage
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := c.ReadJSON(&resp); err != nil {
		t.Fatalf("read hello response: %v", err)
	}
	if resp.SessionID == "" {
		t.Fatal("empty session_id")
	}
	te.SetSessionID(resp.SessionID)
}

func GenerateAdminToken(cfg *config.Config) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"role": "admin",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	signed, _ := token.SignedString([]byte(cfg.Server.InternalToken))
	return signed
}

func ReadWSMessage(t *testing.T, c *websocket.Conn, timeout time.Duration) json.RawMessage {
	c.SetReadDeadline(time.Now().Add(timeout))
	_, r, err := c.NextReader()
	if err != nil {
		t.Fatalf("next reader: %v", err)
	}
	var raw json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return raw
}

func SendWSJSON(t *testing.T, c *websocket.Conn, v any) {
	if err := c.WriteJSON(v); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

func SendWSBinary(t *testing.T, c *websocket.Conn, data []byte) {
	if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("write binary: %v", err)
	}
}

type mockResult struct{ rowsAffected int64 }

func (m *mockResult) LastInsertId() (int64, error) { return 1, nil }
func (m *mockResult) RowsAffected() (int64, error) { return m.rowsAffected, nil }

func MockDeviceRow(deviceID, fwVersion string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"device_id", "user_id", "activation_version", "client_id", "serial_number",
		"firmware_version", "last_seen_at", "activated_at", "token", "created_at",
	}).AddRow(deviceID, 0, 1, "test-client", "", fwVersion, time.Now(), time.Now(), "existing-token", time.Now())
}

func MockDeviceRowSimple(deviceID, fwVersion string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"device_id", "firmware_version", "created_at"}).AddRow(deviceID, fwVersion, time.Now())
}
