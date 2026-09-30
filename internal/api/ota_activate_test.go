package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
)

func setupOTAActivateTest() (*gin.Engine, *mockOTAClient) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mc := &mockOTAClient{}
	h := NewOTAActivateHandler(mc)
	r.GET("/api/device/ota/activate", h.HandleActivate)
	return r, mc
}

func TestOTAActivate_BoundDevice_200(t *testing.T) {
	r, mc := setupOTAActivateTest()
	mc.getDeviceResp = &aisaas.DeviceInfo{DeviceID: "AA:BB:CC:DD:EE:FF", TenantID: 1}
	mc.personaResp = &aisaas.Persona{
		ID:          100,
		PersonaBind: &aisaas.PersonaBind{BindID: 1, PersonaID: 100, DeviceID: "AA:BB:CC:DD:EE:FF"},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/device/ota/activate", nil)
	req.Header.Set("Device-Id", "AA:BB:CC:DD:EE:FF")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestOTAActivate_UnboundDevice_202(t *testing.T) {
	r, mc := setupOTAActivateTest()
	mc.getDeviceResp = &aisaas.DeviceInfo{DeviceID: "AA:BB:CC:DD:EE:FF", TenantID: 1}
	mc.personaErr = aisaas.ErrPersonaNotBound

	req := httptest.NewRequest(http.MethodGet, "/api/device/ota/activate", nil)
	req.Header.Set("Device-Id", "AA:BB:CC:DD:EE:FF")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)
}

func TestOTAActivate_InvalidMAC_400(t *testing.T) {
	r, _ := setupOTAActivateTest()
	req := httptest.NewRequest(http.MethodGet, "/api/device/ota/activate", nil)
	req.Header.Set("Device-Id", "not-a-mac")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOTAActivate_MissingDeviceId_400(t *testing.T) {
	r, _ := setupOTAActivateTest()
	req := httptest.NewRequest(http.MethodGet, "/api/device/ota/activate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOTAActivate_AisaasError_500(t *testing.T) {
	r, mc := setupOTAActivateTest()
	mc.getDeviceErr = errors.New("boom")

	req := httptest.NewRequest(http.MethodGet, "/api/device/ota/activate", nil)
	req.Header.Set("Device-Id", "AA:BB:CC:DD:EE:FF")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}