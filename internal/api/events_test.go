package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
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
