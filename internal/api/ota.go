package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
)

// OTAHandler 对齐 xiaozhi-java DeviceController.ota + DeviceAppService.handleOta：
//   - 校验 Device-Id 为合法 MAC
//   - 设备未开户 → 调 aisaas RegisterDevice → 拿到 bindCode
//   - 设备已开户但未绑 persona → 返回 {"activation":{"code":bindCode,...}}
//   - 设备已绑 persona → 返回 {"websocket":{"url":...,"token":...}}
type OTAHandler struct {
	deviceStore           store.DeviceStore
	aisaasClient          aisaasFullClient
	latestFirmwareVersion string
	publicWSURL           string
}

// aisaasFullClient OTA handler 所需的完整 aisaas 接口（包含注册 + persona 查询）。
type aisaasFullClient interface {
	GetDevice(ctx context.Context, deviceID string) (*aisaas.DeviceInfo, error)
	RegisterDevice(ctx context.Context, deviceID, mac, chipType, firmwareVersion string) (*aisaas.RegisterDeviceResp, error)
	GetPersonaByDevice(ctx context.Context, deviceID string, tenantID int64) (*aisaas.Persona, error)
}

// ActivationInfo 设备激活信息（对齐 xiaozhi-java 响应中的 "activation" 块）。
type ActivationInfo struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Challenge string `json:"challenge"`
}

// WebsocketInfo WebSocket 连接信息。
type WebsocketInfo struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// FirmwareInfo 固件升级信息。
type FirmwareInfo struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Force   bool   `json:"force"`
}

// ServerTime 对齐 xiaozhi-java 响应中的 "server_time" 块（设备用作时钟同步）。
type ServerTime struct {
	Timestamp      int64 `json:"timestamp"`
	TimezoneOffset int   `json:"timezone_offset"`
}

// OTAResponse 对齐 xiaozhi-java 响应：
//   - Websocket + Activation 互斥（同一设备同一时刻只会有一个）
//   - ServerTime / Firmware 视情况返回
type OTAResponse struct {
	Websocket  WebsocketInfo   `json:"websocket,omitempty"`
	Activation *ActivationInfo `json:"activation,omitempty"`
	Firmware   *FirmwareInfo   `json:"firmware,omitempty"`
	ServerTime ServerTime      `json:"server_time"`
}

// OTARequest 设备 OTA 上报 body（对齐 xiaozhi-java parseOtaRequest）。
type OTARequest struct {
	CurrentFirmwareVersion string `json:"current_firmware_version"`
	MACAddress             string `json:"mac_address"`     // fallback for deviceId
	ChipModelName          string `json:"chip_model_name"` // hw info
	Application            *struct {
		Version string `json:"version"`
	} `json:"application"`
	Board *struct {
		SSID string `json:"ssid"`
		Type string `json:"type"`
	} `json:"board"`
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func NewOTAHandler(ds store.DeviceStore, ac aisaasFullClient, latestFW, wsURL string) *OTAHandler {
	return &OTAHandler{
		deviceStore:           ds,
		aisaasClient:          ac,
		latestFirmwareVersion: latestFW,
		publicWSURL:           wsURL,
	}
}

// HandleOTA POST /api/device/ota
//
// 对齐 xiaozhi-java DeviceAppService.handleOta 三态：
//   - 设备未开户 → 自动注册（RegisterDevice）→ 返回 activation code
//   - 设备已开户未绑 persona → 返回 activation code
//   - 设备已绑 persona → 返回 websocket URL
func (h *OTAHandler) HandleOTA(c *gin.Context) {
	deviceID := strings.TrimSpace(c.GetHeader("Device-Id"))
	if !IsMacAddressValid(deviceID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Device-Id: must be a valid unicast MAC address"})
		return
	}
	clientID := strings.TrimSpace(c.GetHeader("Client-Id"))
	if clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing Client-Id header"})
		return
	}
	if strings.TrimSpace(c.GetHeader("Activation-Version")) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing Activation-Version header"})
		return
	}

	var req OTARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	// ---- 1. 查 aisaas：设备开户状态 ----
	devInfo, err := h.aisaasClient.GetDevice(ctx, deviceID)
	if errors.Is(err, aisaas.ErrDeviceNotRegistered) {
		// 设备未开户 → 自动注册
		chipType := req.ChipModelName
		if chipType == "" {
			chipType = "esp32"
		}
		regResp, regErr := h.aisaasClient.RegisterDevice(ctx, deviceID, deviceID, chipType, req.CurrentFirmwareVersion)
		if regErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register device with aisaas: " + regErr.Error()})
			return
		}
		// 注册后重新拉取，拿到 aisaas 写入的 bindCode
		devInfo, err = h.aisaasClient.GetDevice(ctx, deviceID)
		if err != nil || devInfo == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to re-fetch device after registration: " + errMessage(err)})
			return
		}
		devInfo.TenantID = regResp.TenantID
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "aisaas get device failed: " + err.Error()})
		return
	}

	// ---- 2. 本地 device 记录 + token ----
	localDevice, _ := h.deviceStore.GetDevice(ctx, deviceID)
	token := ""
	if localDevice != nil {
		token = localDevice.Token
	}
	if token == "" {
		token = generateToken()
	}

	upsertErr := h.deviceStore.UpsertDevice(ctx, &store.Device{
		DeviceID:        deviceID,
		Token:           token,
		FirmwareVersion: req.CurrentFirmwareVersion,
	})
	if upsertErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upsert device: " + upsertErr.Error()})
		return
	}
	if activateErr := h.deviceStore.ActivateDevice(ctx, deviceID, clientID, ""); activateErr != nil {
		// 非致命：本地激活失败不影响 OTA 响应（设备下次会重试）
		c.Header("X-Activate-Warning", activateErr.Error())
	}

	// ---- 3. 查 persona 绑定状态 ----
	persona, personaErr := h.aisaasClient.GetPersonaByDevice(ctx, deviceID, devInfo.TenantID)
	personaBound := personaErr == nil && persona != nil && persona.PersonaBind != nil

	// ---- 4. 组装响应 ----
	resp := OTAResponse{
		ServerTime: ServerTime{
			Timestamp:      time.Now().UnixMilli(),
			TimezoneOffset: 480, // Asia/Shanghai (+8)
		},
	}

	if personaBound {
		// 已绑定 → 返回 WebSocket 地址（携带新 URL 本身 + token 走设备 WS upgrade 鉴权）
		resp.Websocket = WebsocketInfo{
			URL:   strings.TrimSuffix(h.publicWSURL, "/") + "/ws",
			Token: token,
		}
	} else {
		// 未绑定 → 返回 activation code（bindCode），设备显示给用户
		bindCode := devInfo.BindCode
		if bindCode == "" {
			// 极少见：刚注册完 bindCode 还没回写，下次 OTA 会拿到
			bindCode = deviceID[len(deviceID)-6:]
		}
		resp.Activation = &ActivationInfo{
			Code:      bindCode,
			Message:   bindCode,
			Challenge: deviceID,
		}
	}

	// ---- 5. firmware（升级）----
	if req.CurrentFirmwareVersion != "" && req.CurrentFirmwareVersion != h.latestFirmwareVersion {
		resp.Firmware = &FirmwareInfo{
			Version: h.latestFirmwareVersion,
			URL:     "https://firmware.xiaozhi.ai/" + h.latestFirmwareVersion + "/xiaozhi.bin",
			Force:   false,
		}
	}

	c.JSON(http.StatusOK, resp)
}

func errMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}