package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupProxyTarget() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-Header", "present")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"proxied":true}`))
	}))
}

func TestInternalProxy_ValidJWT_ForwardsRequest(t *testing.T) {
	target := setupProxyTarget()
	defer target.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	secret := "test-secret"

	targetURL, _ := url.Parse(target.URL)
	handler := newReverseProxyHandler(targetURL)

	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.Any("/proxy/*path", handler.ServeHTTP)

	req, _ := http.NewRequest("GET", "/api/internal/v1/proxy/devices/dev1", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT(secret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestInternalProxy_InvalidJWT_401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	secret := "test-secret"

	targetURL, _ := url.Parse("http://localhost:9999")
	handler := newReverseProxyHandler(targetURL)

	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.Any("/proxy/*path", handler.ServeHTTP)

	req, _ := http.NewRequest("GET", "/api/internal/v1/proxy/devices", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestInternalProxy_MissingJWT_401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	secret := "test-secret"

	targetURL, _ := url.Parse("http://localhost:9999")
	handler := newReverseProxyHandler(targetURL)

	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.Any("/proxy/*path", handler.ServeHTTP)

	req, _ := http.NewRequest("GET", "/api/internal/v1/proxy/devices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestInternalProxy_UpstreamError_502(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	secret := "test-secret"

	badURL, _ := url.Parse("http://localhost:99999")
	handler := newReverseProxyHandler(badURL)

	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.Any("/proxy/*path", handler.ServeHTTP)

	req, _ := http.NewRequest("GET", "/api/internal/v1/proxy/devices", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT(secret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadGateway, w.Code)
}

func TestInternalProxy_PathRewrite(t *testing.T) {
	target := setupProxyTarget()
	defer target.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	secret := "test-secret"

	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.GET("/devices", func(c *gin.Context) {
		upstreamURL, _ := url.Parse(target.URL)
		upstreamURL.Path = "/internal/v1/devices"
		h := newReverseProxyHandler(upstreamURL)
		h.ServeHTTP(c)
	})

	req, _ := http.NewRequest("GET", "/api/internal/v1/devices", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT(secret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestInternalProxy_HeaderForward(t *testing.T) {
	target := setupProxyTarget()
	defer target.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	secret := "test-secret"

	targetURL, _ := url.Parse(target.URL)
	handler := newReverseProxyHandler(targetURL)

	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.Any("/proxy/*path", handler.ServeHTTP)

	req, _ := http.NewRequest("GET", "/api/internal/v1/proxy/devices", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT(secret))
	req.Header.Set("X-Request-Id", "req-123")
	req.Header.Set("X-Custom-Header", "custom")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestInternalProxy_ErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	secret := "test-secret"

	badURL, _ := url.Parse("http://localhost:99999")
	handler := newReverseProxyHandler(badURL)

	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.GET("/test", handler.ServeHTTP)

	req, _ := http.NewRequest("GET", "/api/internal/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT(secret))
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadGateway, w.Code)
}

func TestInternalProxy_UpstreamUnreachable_502(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	secret := "test-secret"

	unreachableURL, _ := url.Parse("http://localhost:59999")
	handler := newReverseProxyHandler(unreachableURL)

	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(secret))
	internal.GET("/test", handler.ServeHTTP)

	req, _ := http.NewRequest("GET", "/api/internal/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT(secret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadGateway, w.Code)
}

func TestStripPrefix_RemovesPrefix(t *testing.T) {
	result := stripPrefix("/api/internal/v1/devices", "/api/internal/v1")
	require.Equal(t, "/devices", result)
}

func TestStripPrefix_PreservesRoot(t *testing.T) {
	result := stripPrefix("/other/path", "/api/internal/v1")
	require.Equal(t, "/other/path", result)
}

func TestStripPrefix_EmptyPath(t *testing.T) {
	result := stripPrefix("", "/api/internal/v1")
	require.Equal(t, "", result)
}
