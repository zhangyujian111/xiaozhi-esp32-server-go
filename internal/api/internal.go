package api

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

type ReverseProxyHandler struct {
	target   *url.URL
	director func(*http.Request, *url.URL)
}

func newReverseProxyHandler(target *url.URL) *ReverseProxyHandler {
	return &ReverseProxyHandler{
		target: target,
		director: func(req *http.Request, target *url.URL) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			req.URL.Path = stripPrefix(req.URL.Path, "/api/internal/v1")
		},
	}
}

func stripPrefix(path, prefix string) string {
	if strings.HasPrefix(path, prefix) {
		return path[len(prefix):]
	}
	return path
}

func (h *ReverseProxyHandler) ServeHTTP(c *gin.Context) {
	req := c.Request

	outReq := new(http.Request)
	*outReq = *req
	outReq.Header = make(http.Header)
	copyHeader(outReq.Header, req.Header)

	h.director(outReq, h.target)

	client := &http.Client{}
	resp, err := client.Do(outReq)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			c.Header(k, v)
		}
	}
	c.Status(resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func SetupInternalProxy(upstream string, jwtSecret string) (*gin.Engine, error) {
	target, err := url.Parse(upstream)
	if err != nil {
		return nil, err
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	proxyHandler := newReverseProxyHandler(target)

	internal := r.Group("/api/internal/v1")
	internal.Use(internalJWTAuth(jwtSecret))
	internal.Any("/proxy/*path", proxyHandler.ServeHTTP)

	return r, nil
}
