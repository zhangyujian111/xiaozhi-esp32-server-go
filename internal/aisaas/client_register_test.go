package aisaas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_RegisterDevice_WrappedResponse 验证 RegisterDevice 能正确解析 aisaas 真实响应格式。
//
// aisaas 响应统一包装：{"code":0,"data":{...}}，register 端点的字段在 data 嵌套里。
// 之前的实现 SetResult(&RegisterDeviceResp{}) 直接 unmarshal outer JSON，无法拆 data，
// 导致 TenantID/APIKey/KeyID 全为零值。这是 PR-4 OTA 流程反复返 630452 的 bug 源头。
func TestClient_RegisterDevice_WrappedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/internal/api/v1/devices/device456/register", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"deviceId": "device456",
				"tenantId": 2105304322655916032,
				"apiKey": "sk-aisaas-test-key",
				"keyId": 2105304322655916040
			}
		}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	resp, err := client.RegisterDevice(context.Background(), "device456", "AA:BB:CC:DD:EE:FF", "esp32", "1.0.0")

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "device456", resp.DeviceID, "deviceId 应从 data.deviceId 解出")
	assert.Equal(t, int64(2105304322655916032), resp.TenantID, "tenantId 应从 data.tenantId 解出，不能为 0")
	assert.Equal(t, "sk-aisaas-test-key", resp.APIKey, "apiKey 应从 data.apiKey 解出，不能为空")
	assert.Equal(t, int64(2105304322655916040), resp.KeyID, "keyId 应从 data.keyId 解出，不能为 0")
}

// TestClient_RegisterDevice_WrappedError 验证 code!=0 时返回错误。
func TestClient_RegisterDevice_WrappedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":40404,"message":"设备未开户","data":null}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.RegisterDevice(context.Background(), "device456", "AA:BB:CC:DD:EE:FF", "esp32", "1.0.0")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "code=40404")
}