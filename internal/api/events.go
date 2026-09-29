package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

type SSEHandler struct {
	eventBus EventBusInterface
}

func NewSSEHandler(eb EventBusInterface) *SSEHandler {
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
		case event, ok := <-ch:
			if !ok {
				return false
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
			c.Writer.Flush()
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}
