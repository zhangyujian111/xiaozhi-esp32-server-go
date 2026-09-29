package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/event"
)

func TestSSEHandler_RequiresJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eb := NewEventBus(100)
	h := NewSSEHandler(eb)

	r := gin.New()
	secret := "test-secret"
	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.GET("/events", h.HandleSSE)

	req, _ := http.NewRequest("GET", "/api/internal/v1/events", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSSEHandler_InvalidJWT_401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eb := NewEventBus(100)
	h := NewSSEHandler(eb)

	r := gin.New()
	secret := "test-secret"
	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.GET("/events", h.HandleSSE)

	req, _ := http.NewRequest("GET", "/api/internal/v1/events", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSSEHandler_StreamsEvents_RealServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eb := event.NewEventBus(100)
	h := NewSSEHandler(eb)

	r := gin.New()
	secret := "test-secret"
	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.GET("/events", h.HandleSSE)

	server := httptest.NewServer(r)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		eb.Publish(event.NewDeviceConnectedEvent("dev-123", "sess-456"))
		time.Sleep(50 * time.Millisecond)
		eb.Publish(event.NewDeviceDisconnectedEvent("dev-123", "sess-456"))
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/internal/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT(secret))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lines []string
	timeout := time.After(2 * time.Second)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) >= 8 {
			break
		}
		select {
		case <-timeout:
			t.Fatal("timeout waiting for SSE events")
		default:
		}
	}

	require.GreaterOrEqual(t, len(lines), 4, "expected at least 4 SSE lines")
	eventLine := lines[0]
	require.True(t, strings.HasPrefix(eventLine, "event: "), "expected event: prefix, got: %s", eventLine)
	dataLine := lines[1]
	require.True(t, strings.HasPrefix(dataLine, "data: "), "expected data: prefix, got: %s", dataLine)
	require.Contains(t, dataLine, "device_connected", "expected device_connected event data")
	require.Contains(t, dataLine, "dev-123", "expected device_id in event data")
}
