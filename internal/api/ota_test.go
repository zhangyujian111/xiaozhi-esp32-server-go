package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
)

type mockOTAStore struct {
	devices     map[string]*store.Device
	getErr      error
	upsertErr   error
	activateErr error
}

func (m *mockOTAStore) GetDevice(ctx context.Context, deviceID string) (*store.Device, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if d, ok := m.devices[deviceID]; ok {
		return d, nil
	}
	return nil, store.ErrDeviceNotFound
}

func (m *mockOTAStore) UpsertDevice(ctx context.Context, device *store.Device) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	if device.Token == "" {
		device.Token = "auto-generated-token"
	}
	m.devices[device.DeviceID] = device
	return nil
}

func (m *mockOTAStore) ListDevices(ctx context.Context, limit, offset int) ([]store.Device, error) {
	return nil, nil
}

func (m *mockOTAStore) ActivateDevice(ctx context.Context, deviceID, clientID, serialNumber string) error {
	if m.activateErr != nil {
		return m.activateErr
	}
	if d, ok := m.devices[deviceID]; ok {
		d.ActivationVersion++
		d.ClientID = clientID
		d.SerialNumber = serialNumber
		d.ActivatedAt = time.Now()
		return nil
	}
	return store.ErrDeviceNotFound
}

func (m *mockOTAStore) UpdateDevice(ctx context.Context, device *store.Device) error {
	return nil
}

func (m *mockOTAStore) DeleteDevice(ctx context.Context, deviceID string) error {
	return nil
}

func (m *mockOTAStore) BindDevice(ctx context.Context, deviceID string, userID int64) error {
	return nil
}

func (m *mockOTAStore) UnbindDevice(ctx context.Context, deviceID string) error {
	return nil
}

type mockOTAClient struct {
	getDeviceResp *aisaas.DeviceInfo
	getDeviceErr  error
}

func (m *mockOTAClient) GetDevice(ctx context.Context, deviceID string) (*aisaas.DeviceInfo, error) {
	if m.getDeviceErr != nil {
		return nil, m.getDeviceErr
	}
	return m.getDeviceResp, nil
}

func setupOTATest() (*gin.Engine, *mockOTAStore, *mockOTAClient) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ms := &mockOTAStore{devices: make(map[string]*store.Device)}
	mc := &mockOTAClient{getDeviceResp: &aisaas.DeviceInfo{DeviceID: "dev1", UserID: 1}}
	h := NewOTAHandler(ms, mc, "1.1.0", "wss://example.com/ws")
	r.POST("/api/device/ota", h.HandleOTA)
	return r, ms, mc
}

func TestOTAHandler_Success_NewDevice(t *testing.T) {
	r, _, _ := setupOTATest()

	body := map[string]string{"current_firmware_version": "1.0.0"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/device/ota", bytes.NewReader(b))
	req.Header.Set("Activation-Version", "1")
	req.Header.Set("Device-Id", "dev1")
	req.Header.Set("Client-Id", "clientA")
	req.Header.Set("User-Agent", "ESP32/1.0.0")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp OTAResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "wss://example.com/ws/", resp.Websocket.URL)
	require.NotEmpty(t, resp.Websocket.Token)
	require.NotNil(t, resp.Firmware)
	require.Equal(t, "1.1.0", resp.Firmware.Version)
	require.False(t, resp.Firmware.Force)
}

func TestOTAHandler_Success_ExistingDevice(t *testing.T) {
	r, ms, mc := setupOTATest()
	ms.devices["dev1"] = &store.Device{DeviceID: "dev1", Token: "existing-token", ActivationVersion: 1}
	mc.getDeviceResp = &aisaas.DeviceInfo{DeviceID: "dev1", UserID: 1}

	body := map[string]string{"current_firmware_version": "1.0.0"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/device/ota", bytes.NewReader(b))
	req.Header.Set("Activation-Version", "2")
	req.Header.Set("Device-Id", "dev1")
	req.Header.Set("Client-Id", "clientA")
	req.Header.Set("User-Agent", "ESP32/1.0.0")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp OTAResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "existing-token", resp.Websocket.Token)
	require.NotNil(t, resp.Firmware)
}

func TestOTAHandler_MissingDeviceIDHeader_400(t *testing.T) {
	r, _, _ := setupOTATest()
	body := map[string]string{"current_firmware_version": "1.0.0"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/device/ota", bytes.NewReader(b))
	req.Header.Set("Activation-Version", "1")
	req.Header.Set("Client-Id", "clientA")
	req.Header.Set("User-Agent", "ESP32/1.0.0")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOTAHandler_MissingClientIDHeader_400(t *testing.T) {
	r, _, _ := setupOTATest()
	body := map[string]string{"current_firmware_version": "1.0.0"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/device/ota", bytes.NewReader(b))
	req.Header.Set("Activation-Version", "1")
	req.Header.Set("Device-Id", "dev1")
	req.Header.Set("User-Agent", "ESP32/1.0.0")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOTAHandler_MissingActivationVersionHeader_400(t *testing.T) {
	r, _, _ := setupOTATest()
	body := map[string]string{"current_firmware_version": "1.0.0"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/device/ota", bytes.NewReader(b))
	req.Header.Set("Device-Id", "dev1")
	req.Header.Set("Client-Id", "clientA")
	req.Header.Set("User-Agent", "ESP32/1.0.0")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOTAHandler_DeviceStoreError_500(t *testing.T) {
	r, ms, _ := setupOTATest()
	ms.upsertErr = context.DeadlineExceeded

	body := map[string]string{"current_firmware_version": "1.0.0"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/device/ota", bytes.NewReader(b))
	req.Header.Set("Activation-Version", "1")
	req.Header.Set("Device-Id", "dev1")
	req.Header.Set("Client-Id", "clientA")
	req.Header.Set("User-Agent", "ESP32/1.0.0")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestOTAHandler_FirmwareUpdateNeeded_ReturnsNew(t *testing.T) {
	r, ms, _ := setupOTATest()
	ms.devices["dev1"] = &store.Device{DeviceID: "dev1"}

	body := map[string]string{"current_firmware_version": "1.0.0"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/device/ota", bytes.NewReader(b))
	req.Header.Set("Activation-Version", "1")
	req.Header.Set("Device-Id", "dev1")
	req.Header.Set("Client-Id", "clientA")
	req.Header.Set("User-Agent", "ESP32/1.0.0")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp OTAResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Firmware)
	require.Equal(t, "1.1.0", resp.Firmware.Version)
}

func TestOTAHandler_NoFirmwareUpdate_ReturnsNull(t *testing.T) {
	r, ms, _ := setupOTATest()
	ms.devices["dev1"] = &store.Device{DeviceID: "dev1", FirmwareVersion: "1.1.0"}

	body := map[string]string{"current_firmware_version": "1.1.0"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/device/ota", bytes.NewReader(b))
	req.Header.Set("Activation-Version", "1")
	req.Header.Set("Device-Id", "dev1")
	req.Header.Set("Client-Id", "clientA")
	req.Header.Set("User-Agent", "ESP32/1.0.0")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp OTAResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Nil(t, resp.Firmware)
}

func TestOTAHandler_AutoAssignsToken(t *testing.T) {
	r, _, _ := setupOTATest()

	body := map[string]string{"current_firmware_version": "1.0.0"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/device/ota", bytes.NewReader(b))
	req.Header.Set("Activation-Version", "1")
	req.Header.Set("Device-Id", "newdev")
	req.Header.Set("Client-Id", "clientA")
	req.Header.Set("User-Agent", "ESP32/1.0.0")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp OTAResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Websocket.Token)
	require.Len(t, resp.Websocket.Token, 32)
}
