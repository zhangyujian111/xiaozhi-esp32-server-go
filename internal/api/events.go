package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/event"
)

type SSEHandler struct {
	eventBus event.EventBusInterface
}

func NewSSEHandler(eb event.EventBusInterface) *SSEHandler {
	return &SSEHandler{eventBus: eb}
}

func (h *SSEHandler) HandleSSE(c *gin.Context) {
	ch, unsub := h.eventBus.Subscribe(nil)
	defer unsub()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Status(http.StatusOK)

	c.Stream(func(w io.Writer) bool {
		select {
		case ev, ok := <-ch:
			if !ok {
				return false
			}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
			c.Writer.Flush()
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}
