package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
)

// OTAActivateHandler 设备轮询"我激活了吗"端点（对齐 xiaozhi-java DeviceController.otaActivate）。
//
// 设备收到 activation code 并显示给用户后，会反复调用本端点：
//   - 已绑 persona → 200 OK（设备可重新调 POST /api/device/ota 拿 websocket URL）
//   - 未绑 persona → 202 Accepted（继续等）
//   - aisaas 5xx → 500
//
// 设备在连续 202 后应使用指数退避，避免打爆 aisaas。
type OTAActivateHandler struct {
	aisaasClient aisaasActivateClient
}

// aisaasActivateClient 仅需 GetDevice + GetPersonaByDevice。
type aisaasActivateClient interface {
	GetDevice(ctx context.Context, deviceID string) (*aisaas.DeviceInfo, error)
	GetPersonaByDevice(ctx context.Context, deviceID string, tenantID int64) (*aisaas.Persona, error)
}

func NewOTAActivateHandler(ac aisaasActivateClient) *OTAActivateHandler {
	return &OTAActivateHandler{aisaasClient: ac}
}

// HandleActivate GET /api/device/ota/activate
func (h *OTAActivateHandler) HandleActivate(c *gin.Context) {
	deviceID := strings.TrimSpace(c.GetHeader("Device-Id"))
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing Device-Id header"})
		return
	}
	if !IsMacAddressValid(deviceID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Device-Id: must be a valid unicast MAC address"})
		return
	}

	ctx := c.Request.Context()

	// 查 aisaas：拿 tenantId + bindCode（顺便校验设备已开户）
	devInfo, err := h.aisaasClient.GetDevice(ctx, deviceID)
	if err != nil {
		if errors.Is(err, aisaas.ErrDeviceNotRegistered) {
			// 设备未开户 → 视同未激活（用户需先调 POST /api/device/ota 完成自动注册）
			c.Status(http.StatusAccepted)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "aisaas get device failed: " + err.Error()})
		return
	}

	// 查 persona 绑定状态
	persona, err := h.aisaasClient.GetPersonaByDevice(ctx, deviceID, devInfo.TenantID)
	if err != nil {
		if errors.Is(err, aisaas.ErrPersonaNotBound) {
			c.Status(http.StatusAccepted)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "aisaas get persona failed: " + err.Error()})
		return
	}

	if persona == nil || persona.PersonaBind == nil {
		c.Status(http.StatusAccepted)
		return
	}

	c.Status(http.StatusOK)
}