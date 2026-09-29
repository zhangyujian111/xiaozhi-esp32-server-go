package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
)

type mockDeviceStore struct {
	devices    map[string]*store.Device
	getErr     error
	listErr    error
	updateErr  error
	deleteErr  error
	bindErr    error
	unbindErr  error
	listResult []store.Device
}

func (m *mockDeviceStore) GetDevice(ctx context.Context, deviceID string) (*store.Device, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if d, ok := m.devices[deviceID]; ok {
		return d, nil
	}
	return nil, store.ErrDeviceNotFound
}

func (m *mockDeviceStore) UpsertDevice(ctx context.Context, device *store.Device) error {
	return nil
}

func (m *mockDeviceStore) ListDevices(ctx context.Context, limit, offset int) ([]store.Device, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	if m.listResult != nil {
		result := m.listResult
		if offset >= len(result) {
			return []store.Device{}, nil
		}
		end := offset + limit
		if end > len(result) {
			end = len(result)
		}
		return result[offset:end], nil
	}
	var result []store.Device
	for _, d := range m.devices {
		result = append(result, *d)
	}
	if offset >= len(result) {
		return []store.Device{}, nil
	}
	end := offset + limit
	if end > len(result) {
		end = len(result)
	}
	return result[offset:end], nil
}

func (m *mockDeviceStore) ActivateDevice(ctx context.Context, deviceID, clientID, serialNumber string) error {
	return nil
}

func (m *mockDeviceStore) UpdateDevice(ctx context.Context, device *store.Device) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if _, ok := m.devices[device.DeviceID]; !ok {
		return store.ErrDeviceNotFound
	}
	m.devices[device.DeviceID] = device
	return nil
}

func (m *mockDeviceStore) DeleteDevice(ctx context.Context, deviceID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.devices[deviceID]; !ok {
		return store.ErrDeviceNotFound
	}
	delete(m.devices, deviceID)
	return nil
}

func (m *mockDeviceStore) BindDevice(ctx context.Context, deviceID string, userID int64) error {
	if m.bindErr != nil {
		return m.bindErr
	}
	if _, ok := m.devices[deviceID]; !ok {
		return store.ErrDeviceNotFound
	}
	m.devices[deviceID].UserID = userID
	return nil
}

func (m *mockDeviceStore) UnbindDevice(ctx context.Context, deviceID string) error {
	if m.unbindErr != nil {
		return m.unbindErr
	}
	if _, ok := m.devices[deviceID]; !ok {
		return store.ErrDeviceNotFound
	}
	m.devices[deviceID].UserID = 0
	return nil
}

func setupDeviceRouter() (*gin.Engine, *mockDeviceStore) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ms := &mockDeviceStore{devices: make(map[string]*store.Device)}
	dh := NewDeviceHandler(ms)

	secret := "test-internal-jwt-secret"
	internalGroup := r.Group("/api/internal/v1")
	internalGroup.Use(internalJWTAuth(secret))
	internalGroup.GET("/devices", dh.ListDevices)
	internalGroup.GET("/devices/:deviceID", dh.GetDevice)
	internalGroup.POST("/devices/:deviceID/bind", dh.BindDevice)
	internalGroup.POST("/devices/:deviceID/unbind", dh.UnbindDevice)
	internalGroup.DELETE("/devices/:deviceID", dh.DeleteDevice)

	return r, ms
}

func makeAdminJWT(secret string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"role": "admin",
	})
	tokenString, _ := token.SignedString([]byte(secret))
	return tokenString
}

func TestDeviceCRUD_ListDevices_Pagination(t *testing.T) {
	r, ms := setupDeviceRouter()
	ms.listResult = []store.Device{
		{DeviceID: "dev1", UserID: 1},
		{DeviceID: "dev2", UserID: 2},
	}

	req, _ := http.NewRequest("GET", "/api/internal/v1/devices?limit=1&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("test-internal-jwt-secret"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Devices []store.Device `json:"devices"`
		Total   int            `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Devices, 1)
}

func TestDeviceCRUD_GetDevice_NotFound_404(t *testing.T) {
	r, _ := setupDeviceRouter()

	req, _ := http.NewRequest("GET", "/api/internal/v1/devices/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("test-internal-jwt-secret"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeviceCRUD_GetDevice_Success(t *testing.T) {
	r, ms := setupDeviceRouter()
	ms.devices["dev1"] = &store.Device{DeviceID: "dev1", UserID: 42}

	req, _ := http.NewRequest("GET", "/api/internal/v1/devices/dev1", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("test-internal-jwt-secret"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp store.Device
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "dev1", resp.DeviceID)
	require.Equal(t, int64(42), resp.UserID)
}

func TestDeviceCRUD_BindDevice_Success_200(t *testing.T) {
	r, ms := setupDeviceRouter()
	ms.devices["dev1"] = &store.Device{DeviceID: "dev1", UserID: 0}

	body := map[string]int64{"user_id": 123}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/internal/v1/devices/dev1/bind", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("test-internal-jwt-secret"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(123), ms.devices["dev1"].UserID)
}

func TestDeviceCRUD_UnbindDevice_Success_204(t *testing.T) {
	r, ms := setupDeviceRouter()
	ms.devices["dev1"] = &store.Device{DeviceID: "dev1", UserID: 42}

	req, _ := http.NewRequest("POST", "/api/internal/v1/devices/dev1/unbind", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("test-internal-jwt-secret"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, int64(0), ms.devices["dev1"].UserID)
}

func TestDeviceCRUD_DeleteActiveDevice_403(t *testing.T) {
	r, ms := setupDeviceRouter()
	ms.devices["dev1"] = &store.Device{DeviceID: "dev1", ActivationVersion: 1}

	req, _ := http.NewRequest("DELETE", "/api/internal/v1/devices/dev1", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("test-internal-jwt-secret"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestDeviceCRUD_DeleteInactiveDevice_Success(t *testing.T) {
	r, ms := setupDeviceRouter()
	ms.devices["dev1"] = &store.Device{DeviceID: "dev1", ActivationVersion: 0}

	req, _ := http.NewRequest("DELETE", "/api/internal/v1/devices/dev1", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("test-internal-jwt-secret"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	_, exists := ms.devices["dev1"]
	require.False(t, exists)
}

func TestInternalJWT_ValidToken_Allows(t *testing.T) {
	r, _ := setupDeviceRouter()

	req, _ := http.NewRequest("GET", "/api/internal/v1/devices", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("test-internal-jwt-secret"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestInternalJWT_InvalidToken_401(t *testing.T) {
	r, _ := setupDeviceRouter()

	req, _ := http.NewRequest("GET", "/api/internal/v1/devices", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestInternalJWT_MissingHeader_401(t *testing.T) {
	r, _ := setupDeviceRouter()

	req, _ := http.NewRequest("GET", "/api/internal/v1/devices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestInternalJWT_NonAdminRole_403(t *testing.T) {
	r, _ := setupDeviceRouter()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"role": "user",
	})
	tokenString, _ := token.SignedString([]byte("test-internal-jwt-secret"))

	req, _ := http.NewRequest("GET", "/api/internal/v1/devices", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestDeviceCRUD_BindDevice_DeviceNotFound_404(t *testing.T) {
	r, ms := setupDeviceRouter()
	ms.getErr = store.ErrDeviceNotFound

	body := map[string]int64{"user_id": 123}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/internal/v1/devices/nonexistent/bind", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("test-internal-jwt-secret"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeviceCRUD_BindDevice_InvalidUserID_400(t *testing.T) {
	r, _ := setupDeviceRouter()

	req, _ := http.NewRequest("POST", "/api/internal/v1/devices/dev1/bind", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("test-internal-jwt-secret"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeviceCRUD_UnbindDevice_DeviceNotFound_404(t *testing.T) {
	r, ms := setupDeviceRouter()
	ms.unbindErr = store.ErrDeviceNotFound

	req, _ := http.NewRequest("POST", "/api/internal/v1/devices/nonexistent/unbind", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("test-internal-jwt-secret"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeviceCRUD_DeleteDevice_DeviceNotFound_404(t *testing.T) {
	r, ms := setupDeviceRouter()
	ms.getErr = store.ErrDeviceNotFound

	req, _ := http.NewRequest("DELETE", "/api/internal/v1/devices/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+makeAdminJWT("test-internal-jwt-secret"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}
