package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
)

type OTAHandler struct {
	deviceStore           store.DeviceStore
	aisaasClient          aisaasClient
	latestFirmwareVersion string
	publicWSURL           string
}

type aisaasClient interface {
	GetDevice(ctx context.Context, deviceID string) (*aisaas.DeviceInfo, error)
}

type OTAResponse struct {
	Websocket struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	} `json:"websocket"`
	Firmware *FirmwareInfo `json:"firmware"`
}

type FirmwareInfo struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Force   bool   `json:"force"`
}

type OTARequest struct {
	CurrentFirmwareVersion string `json:"current_firmware_version"`
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func NewOTAHandler(ds store.DeviceStore, ac aisaasClient, latestFW, wsURL string) *OTAHandler {
	return &OTAHandler{
		deviceStore:           ds,
		aisaasClient:          ac,
		latestFirmwareVersion: latestFW,
		publicWSURL:           wsURL,
	}
}

func (h *OTAHandler) HandleOTA(c *gin.Context) {
	deviceID := strings.TrimSpace(c.GetHeader("Device-Id"))
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing Device-Id header"})
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

	existing, _ := h.deviceStore.GetDevice(context.Background(), deviceID)
	token := ""
	if existing != nil {
		token = existing.Token
	}
	if token == "" {
		token = generateToken()
	}

	device := &store.Device{
		DeviceID:        deviceID,
		Token:           token,
		FirmwareVersion: req.CurrentFirmwareVersion,
	}
	if err := h.deviceStore.UpsertDevice(context.Background(), device); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upsert device"})
		return
	}

	if err := h.deviceStore.ActivateDevice(context.Background(), deviceID, clientID, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate device"})
		return
	}

	_, aisaasErr := h.aisaasClient.GetDevice(context.Background(), deviceID)
	_ = aisaasErr

	wsURL := strings.TrimSuffix(h.publicWSURL, "/")

	var firmware *FirmwareInfo
	if req.CurrentFirmwareVersion != "" && req.CurrentFirmwareVersion != h.latestFirmwareVersion {
		firmware = &FirmwareInfo{
			Version: h.latestFirmwareVersion,
			URL:     "https://firmware.xiaozhi.ai/" + h.latestFirmwareVersion + "/xiaozhi.bin",
			Force:   false,
		}
	}

	resp := OTAResponse{}
	resp.Websocket.URL = wsURL + "/"
	resp.Websocket.Token = token
	resp.Firmware = firmware

	c.JSON(http.StatusOK, resp)
}
