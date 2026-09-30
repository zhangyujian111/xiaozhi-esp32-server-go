package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	getDeviceResp     *aisaas.DeviceInfo
	getDeviceErr      error
	getDeviceCalls    int
	registerCalled    bool
	registerResp      *aisaas.RegisterDeviceResp
	registerErr       error
	personaResp       *aisaas.Persona
	personaErr        error
}

func (m *mockOTAClient) GetDevice(ctx context.Context, deviceID string) (*aisaas.DeviceInfo, error) {
	m.getDeviceCalls++
	if m.getDeviceCalls == 1 && m.getDeviceErr != nil {
		return nil, m.getDeviceErr
	}
	if m.getDeviceResp == nil {
		return nil, aisaas.ErrDeviceNotRegistered
	}
	return m.getDeviceResp, nil
}

func (m *mockOTAClient) RegisterDevice(ctx context.Context, deviceID, mac, chipType, firmwareVersion string) (*aisaas.RegisterDeviceResp, error) {
	m.registerCalled = true
	if m.registerErr != nil {
		return nil, m.registerErr
	}
	return m.registerResp, nil
}

func (m *mockOTAClient) GetPersonaByDevice(ctx context.Context, deviceID string, tenantID int64) (*aisaas.Persona, error) {
	if m.personaErr != nil {
		return nil, m.personaErr
	}
	return m.personaResp, nil
}

func setupOTATest() (*gin.Engine, *mockOTAStore, *mockOTAClient) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ms := &mockOTAStore{devices: make(map[string]*store.Device)}
	mc := &mockOTAClient{getDeviceResp: &aisaas.DeviceInfo{DeviceID: "AA:BB:CC:DD:EE:FF", TenantID: 1, BindCode: "123456"}}
	h := NewOTAHandler(ms, mc, "1.1.0", "ws://192.168.200.242:8082")
	r.POST("/api/device/ota", h.HandleOTA)
	return r, ms, mc
}

// 标准 MAC + 完整头部 + body
func otaRequest(t *testing.T, r http.Handler, deviceID, currentFW string) *httptest.ResponseRecorder {
	t.Helper()
	body := map[string]string{"current_firmware_version": currentFW}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/device/ota", bytes.NewReader(b))
	req.Header.Set("Activation-Version", "1")
	req.Header.Set("Device-Id", deviceID)
	req.Header.Set("Client-Id", "clientA")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ===== MAC 校验 =====

func TestOTA_InvalidMAC_400(t *testing.T) {
	r, _, _ := setupOTATest()
	w := otaRequest(t, r, "not-a-mac", "1.0.0")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "invalid Device-Id")
}

// ===== 设备未开户 → 自动注册 → 返回 activation =====

func TestOTA_UnregisteredDevice_AutoRegistersAndReturnsActivation(t *testing.T) {
	r, ms, mc := setupOTATest()
	mc.getDeviceErr = aisaas.ErrDeviceNotRegistered
	mc.registerResp = &aisaas.RegisterDeviceResp{DeviceID: "AA:BB:CC:DD:EE:FF", TenantID: 1001, APIKey: "sk-test", KeyID: 99}
	// After registration, GetDevice should return bindCode
	mc.getDeviceErr = errors.New("first call: not registered")
	// Override mock: first call returns ErrDeviceNotRegistered, second returns success
	mc = &mockOTAClient{
		getDeviceResp: &aisaas.DeviceInfo{DeviceID: "AA:BB:CC:DD:EE:FF", TenantID: 1001, BindCode: "654321"},
	}
	// rebuild handler with new mock
	gin.SetMode(gin.TestMode)
	r = gin.New()
	h := NewOTAHandler(ms, mc, "1.1.0", "ws://192.168.200.242:8082")
	r.POST("/api/device/ota", h.HandleOTA)

	mc.getDeviceErr = aisaas.ErrDeviceNotRegistered
	mc.registerResp = &aisaas.RegisterDeviceResp{DeviceID: "AA:BB:CC:DD:EE:FF", TenantID: 1001, APIKey: "sk-test", KeyID: 99}

	w := otaRequest(t, r, "AA:BB:CC:DD:EE:FF", "1.0.0")
	require.Equal(t, http.StatusOK, w.Code)

	var resp OTAResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Activation)
	assertEqual(t, "654321", resp.Activation.Code)
	assertEqual(t, "654321", resp.Activation.Message)
	assertEqual(t, "AA:BB:CC:DD:EE:FF", resp.Activation.Challenge)
	assertEqual(t, "", resp.Websocket.URL, "未绑 persona 时不应返回 websocket URL")
	assertTrue(t, mc.registerCalled, "首次调用应触发 RegisterDevice")
}

// ===== 已开户未绑 persona → 返回 activation =====

func TestOTA_RegisteredDevice_NoPersonaBound_ReturnsActivation(t *testing.T) {
	r, _, mc := setupOTATest()
	// bindCode 已有但 persona 未绑
	mc.personaErr = aisaas.ErrPersonaNotBound

	w := otaRequest(t, r, "AA:BB:CC:DD:EE:FF", "1.0.0")
	require.Equal(t, http.StatusOK, w.Code)

	var resp OTAResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Activation)
	assertEqual(t, "123456", resp.Activation.Code)
	assertEqual(t, "", resp.Websocket.URL)
}

// ===== 已绑 persona → 返回 websocket =====

func TestOTA_RegisteredDevice_PersonaBound_ReturnsWebSocket(t *testing.T) {
	r, _, mc := setupOTATest()
	mc.personaResp = &aisaas.Persona{
		ID:          100,
		Code:        "hakumi02",
		Name:        "hakumi",
		TenantID:    1,
		PersonaBind: &aisaas.PersonaBind{BindID: 1, PersonaID: 100, DeviceID: "AA:BB:CC:DD:EE:FF", IsDefault: true},
	}

	w := otaRequest(t, r, "AA:BB:CC:DD:EE:FF", "1.0.0")
	require.Equal(t, http.StatusOK, w.Code)

	var resp OTAResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assertEqual(t, "ws://192.168.200.242:8082/ws", resp.Websocket.URL)
	assertNil(t, resp.Activation)
}

// ===== 已有本地 device + 绑 persona → 复用 token =====

func TestOTA_ExistingLocalDevice_Bound_PreservesToken(t *testing.T) {
	r, ms, mc := setupOTATest()
	ms.devices["AA:BB:CC:DD:EE:FF"] = &store.Device{
		DeviceID: "AA:BB:CC:DD:EE:FF",
		Token:    "existing-token-1234",
	}
	mc.personaResp = &aisaas.Persona{
		ID:          100,
		Code:        "hakumi02",
		TenantID:    1,
		PersonaBind: &aisaas.PersonaBind{BindID: 1, PersonaID: 100, DeviceID: "AA:BB:CC:DD:EE:FF"},
	}

	w := otaRequest(t, r, "AA:BB:CC:DD:EE:FF", "1.0.0")
	require.Equal(t, http.StatusOK, w.Code)

	var resp OTAResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assertEqual(t, "existing-token-1234", resp.Websocket.Token)
}

// ===== Firmware 升级 =====

func TestOTA_FirmwareUpdateNeeded_ReturnsNew(t *testing.T) {
	r, _, mc := setupOTATest()
	mc.personaResp = &aisaas.Persona{
		ID:          100,
		PersonaBind: &aisaas.PersonaBind{BindID: 1, PersonaID: 100, DeviceID: "AA:BB:CC:DD:EE:FF"},
	}

	w := otaRequest(t, r, "AA:BB:CC:DD:EE:FF", "1.0.0")
	require.Equal(t, http.StatusOK, w.Code)

	var resp OTAResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Firmware)
	assertEqual(t, "1.1.0", resp.Firmware.Version)
}

func TestOTA_NoFirmwareUpdate_ReturnsNull(t *testing.T) {
	r, _, mc := setupOTATest()
	mc.personaResp = &aisaas.Persona{
		ID:          100,
		PersonaBind: &aisaas.PersonaBind{BindID: 1, PersonaID: 100, DeviceID: "AA:BB:CC:DD:EE:FF"},
	}

	w := otaRequest(t, r, "AA:BB:CC:DD:EE:FF", "1.1.0")
	require.Equal(t, http.StatusOK, w.Code)

	var resp OTAResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assertNil(t, resp.Firmware)
}

// ===== 错误处理 =====

func TestOTA_AisaasGetDevice_500(t *testing.T) {
	r, _, mc := setupOTATest()
	mc.getDeviceErr = errors.New("aisaas 500")

	w := otaRequest(t, r, "AA:BB:CC:DD:EE:FF", "1.0.0")
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

// ===== helper =====

func assertEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	require.Equal(t, expected, actual, msgAndArgs...)
}

func assertTrue(t *testing.T, cond bool, msg string) {
	t.Helper()
	require.True(t, cond, msg)
}

func assertNil(t *testing.T, val interface{}) {
	t.Helper()
	require.Nil(t, val)
}