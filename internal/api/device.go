package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
)

type DeviceHandler struct {
	deviceStore store.DeviceStore
}

func NewDeviceHandler(ds store.DeviceStore) *DeviceHandler {
	return &DeviceHandler{deviceStore: ds}
}

func (h *DeviceHandler) ListDevices(c *gin.Context) {
	limit := parseIntParam(c.DefaultQuery("limit", "20"), 20)
	offset := parseIntParam(c.DefaultQuery("offset", "0"), 0)

	devices, err := h.deviceStore.ListDevices(context.Background(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list devices"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"devices": devices,
		"total":   len(devices),
		"limit":   limit,
		"offset":  offset,
	})
}

func (h *DeviceHandler) GetDevice(c *gin.Context) {
	deviceID := c.Param("deviceID")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing deviceID"})
		return
	}

	device, err := h.deviceStore.GetDevice(context.Background(), deviceID)
	if err == store.ErrDeviceNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get device"})
		return
	}

	c.JSON(http.StatusOK, device)
}

func (h *DeviceHandler) BindDevice(c *gin.Context) {
	deviceID := c.Param("deviceID")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing deviceID"})
		return
	}

	var req struct {
		UserID int64 `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := h.deviceStore.BindDevice(context.Background(), deviceID, req.UserID); err == store.ErrDeviceNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to bind device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "bound"})
}

func (h *DeviceHandler) UnbindDevice(c *gin.Context) {
	deviceID := c.Param("deviceID")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing deviceID"})
		return
	}

	if err := h.deviceStore.UnbindDevice(context.Background(), deviceID); err == store.ErrDeviceNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unbind device"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *DeviceHandler) DeleteDevice(c *gin.Context) {
	deviceID := c.Param("deviceID")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing deviceID"})
		return
	}

	device, err := h.deviceStore.GetDevice(context.Background(), deviceID)
	if err == store.ErrDeviceNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get device"})
		return
	}

	if device.ActivationVersion > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete activated device"})
		return
	}

	if err := h.deviceStore.DeleteDevice(context.Background(), deviceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete device"})
		return
	}

	c.Status(http.StatusNoContent)
}

func parseIntParam(s string, defaultVal int) int {
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return defaultVal
	}
	return v
}
