package aisaas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

var ErrNotImplemented = errors.New("not implemented yet")

type Client struct {
	http          *resty.Client
	baseURL       string
	internalToken string
}

func NewClient(baseURL, internalToken string) *Client {
	c := resty.New().
		SetTimeout(30*time.Second).
		SetHeader("X-Internal-Token", internalToken)
	return &Client{http: c, baseURL: baseURL, internalToken: internalToken}
}

type DeviceInfo struct {
	DeviceID    string `json:"deviceId"`
	UserID      int64  `json:"userId"`
	RoleID      int64  `json:"roleId"`
	TenantState string `json:"tenantState"`
	LLMConfigID int    `json:"llmConfigId"`
	TTSConfigID int    `json:"ttsConfigId"`
	STTConfigID int    `json:"sttConfigId"`
	VoiceName   string `json:"voiceName"`

	// PR-4 新增：bind-code 端点返回的实际绑定码（设备显示给用户）
	// TenantID 对应 aisaas 内部租户 ID（设备开户时的租户）
	BindCode string `json:"bindCode"`
	TenantID int64  `json:"tenantId"`
}

// PersonaBind 对齐 aisaas PersonaBindResp（设备↔人设绑定记录）。
// nil 表示该 persona 未绑定到指定设备。
type PersonaBind struct {
	BindID    int64  `json:"bindId,string"`
	PersonaID int64  `json:"personaId,string"`
	DeviceID  string `json:"deviceId"`
	IsDefault bool   `json:"isDefault"`
	BoundAt   string `json:"boundAt"`
}

// Persona 对齐 aisaas PersonaResp（按字段子集：orchestrator 仅用 system prompt + 元数据）。
//
// PR-4 增量：PersonaBind 字段携带设备↔人设绑定记录。
// OTA handler 用 `Persona.PersonaBind != nil` 判定"设备是否绑了 persona"。
type Persona struct {
	ID         int64  `json:"id,string"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	ModelType  string `json:"modelType"`
	TenantID   int64  `json:"tenantId,string"`
	SystemPrompt string `json:"systemPrompt"`
	DefaultModelID string `json:"defaultModelId"`
	Temperature    float64 `json:"temperature"`
	TopP           float64 `json:"topP"`
	MaxTokens      int     `json:"maxTokens"`
	MemoryType     string  `json:"memoryType"`
	PersonaBind    *PersonaBind `json:"persona_bind,omitempty"`
}

func (c *Client) GetDevice(ctx context.Context, deviceID string) (*DeviceInfo, error) {
	// aisaas 响应包装：{"code":0,"data":{"bindCode":"...","deviceId":"...","tenantId":N}}
	// resty SetResult 不会自动拆 data，需要手动两段解析
	var outer struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	resp, err := c.http.R().
		SetContext(ctx).
		SetHeader("X-Device-Id", deviceID).
		SetResult(&outer).
		Get(c.baseURL + "/internal/api/v1/devices/" + deviceID + "/bind-code")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, ErrDeviceNotRegistered
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get device failed: status=%d body=%s", resp.StatusCode(), resp.String())
	}
	var bindResp struct {
		DeviceID string `json:"deviceId"`
		TenantID int64  `json:"tenantId"`
		BindCode string `json:"bindCode"`
	}
	if err := json.Unmarshal(outer.Data, &bindResp); err != nil {
		return nil, fmt.Errorf("parse device bind data: %w", err)
	}
	return &DeviceInfo{
		DeviceID: bindResp.DeviceID,
		TenantID: bindResp.TenantID,
		UserID:   bindResp.TenantID, // 历史语义：userId 实际指 tenantId
		BindCode: bindResp.BindCode,
	}, nil
}

var ErrDeviceNotRegistered = fmt.Errorf("device not registered with aisaas")

// RegisterDeviceResp 对齐 aisaas RegisterDeviceResp（设备注册响应）。
type RegisterDeviceResp struct {
	DeviceID string `json:"deviceId"`
	TenantID int64  `json:"tenantId"`
	APIKey   string `json:"apiKey"`
	KeyID    int64  `json:"keyId"`
}

// RegisterDevice POST /internal/api/v1/devices/{deviceId}/register
//
// 设备首次连接 OTA 时调用此端点：
//  1. aisaas 创建（或幂等获取）设备租户
//  2. 为该租户签发一个专属 API Key
//  3. 返回明文 Key（xz 加密保存到本地 keystore —— 本 PR 不持久化）
//
// 对齐 xiaozhi-java DeviceAppService.handleOta 的"设备未开户 → 调注册"分支。
func (c *Client) RegisterDevice(ctx context.Context, deviceID, mac, chipType, firmwareVersion string) (*RegisterDeviceResp, error) {
	body := map[string]interface{}{
		"hwInfo": map[string]string{
			"mac":             mac,
			"chipType":        chipType,
			"firmwareVersion": firmwareVersion,
		},
	}
	resp, err := c.http.R().
		SetContext(ctx).
		SetHeader("X-Device-Id", deviceID).
		SetBody(body).
		SetResult(&RegisterDeviceResp{}).
		Post(c.baseURL + "/internal/api/v1/devices/" + deviceID + "/register")
	if err != nil {
		return nil, fmt.Errorf("register device: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("register device failed: status=%d body=%s", resp.StatusCode(), resp.String())
	}
	return resp.Result().(*RegisterDeviceResp), nil
}

// ErrPersonaNotBound 设备未绑定 persona；orchestrator 应回退到默认 prompt，不应让用户感知错误。
var ErrPersonaNotBound = fmt.Errorf("device has no persona bound")

func (c *Client) VerifyDeviceToken(ctx context.Context, deviceID, token string) error {
	if token == "" {
		return fmt.Errorf("empty token")
	}
	_, err := c.GetDevice(ctx, deviceID)
	return err
}

// GetPersonaByDevice 拉取绑定到指定设备的 Persona（aisaas 内部 API）。
// 返回 ErrPersonaNotBound（40404）表示设备未绑定 persona，由调用方决定是否回退默认 prompt。
//
// aisaas 返回包装：{"code":0,"data":[{"...persona fields...","persona_bind":{...}}]}
// 这里跳到 data 数组第一个元素再解。
func (c *Client) GetPersonaByDevice(ctx context.Context, deviceID string, tenantID int64) (*Persona, error) {
	url := fmt.Sprintf("%s/internal/api/v1/personas/by-device?deviceId=%s&tenantId=%d", c.baseURL, deviceID, tenantID)
	resp, err := c.http.R().
		SetContext(ctx).
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("get persona by device: %w", err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, ErrPersonaNotBound
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get persona failed: status=%d body=%s", resp.StatusCode(), resp.String())
	}
	var outer struct {
		Code    int               `json:"code"`
		Data    []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &outer); err != nil {
		return nil, fmt.Errorf("parse persona response: %w", err)
	}
	if outer.Code != 0 || len(outer.Data) == 0 {
		return nil, ErrPersonaNotBound
	}
	var p Persona
	if err := json.Unmarshal(outer.Data[0], &p); err != nil {
		return nil, fmt.Errorf("parse persona element: %w", err)
	}
	return &p, nil
}

type Authenticator interface {
	VerifyDeviceToken(ctx context.Context, deviceID, token string) error
}

var _ Authenticator = (*Client)(nil)
