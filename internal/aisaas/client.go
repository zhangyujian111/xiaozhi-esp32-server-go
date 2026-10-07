package aisaas

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

var ErrNotImplemented = errors.New("not implemented yet")

type Client struct {
	http          *resty.Client
	baseURL       string
	internalToken string
	apiKey        string // 当前请求使用的设备 API Key（Bearer sk-aisaas-...）。由 SetAPIKey / applyAPIKey 注入。
}

func NewClient(baseURL, internalToken string) *Client {
	c := resty.New().
		SetTimeout(30*time.Second).
		SetHeader("X-Internal-Token", internalToken)
	return &Client{http: c, baseURL: baseURL, internalToken: internalToken}
}

// SetAPIKey 设置当前 Client 默认使用的设备 API Key（Bearer）。
// 调用 STT/Chat/TTS/GetPersonaByDevice 时会自动附加。
// 单机单设备场景下使用；多设备并发请使用 CloneAPIKey。
func (c *Client) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}

// CloneAPIKey 返回一个共享 http client、但持有独立 apiKey 的浅拷贝。
// 用于多设备并发：每个 session/handler 持有自己的 clone，互不干扰。
func (c *Client) CloneAPIKey(apiKey string) *Client {
	return &Client{
		http:          c.http,
		baseURL:       c.baseURL,
		internalToken: c.internalToken,
		apiKey:        apiKey,
	}
}

// applyAPIKey 把当前 Client.apiKey 注入到 resty Request 的 Authorization 头。
// apiKey 为空时跳过（兼容不需 Bearer 的内部端点）。
func (c *Client) applyAPIKey(r *resty.Request) {
	if c.apiKey != "" {
		r.SetHeader("Authorization", "Bearer "+c.apiKey)
	}
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

	// APIKey 由 ws.handler 在调用 STT/Chat/TTS 前注入，
	// orchestrator 透传给 aisaas.Client（per-request Bearer header）。
	APIKey string `json:"apiKey,omitempty"`
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
	TTSProvider    string `json:"ttsProvider"`
	TTSVoice       string `json:"ttsVoice"`
	VoicePreference *VoicePreference `json:"voicePreference,omitempty"`
	PersonaBind    *PersonaBind `json:"persona_bind,omitempty"`
}

type VoicePreference struct {
	Pitch  float64 `json:"pitch"`
	Speed  float64 `json:"speed"`
	Voice  string  `json:"voice"`
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

// ErrQuotaExceeded dialogue quota exhausted for tenant.
var ErrQuotaExceeded = fmt.Errorf("dialogue quota exceeded")

// ErrModelNotFound STT/LLM/TTS model not found or not enabled.
var ErrModelNotFound = fmt.Errorf("model not found or not enabled")

// DialogueResult 对齐 aisaas POST /internal/api/v1/dialogue/run 响应。
type DialogueResult struct {
	UserText    string `json:"userText"`
	ReplyText   string `json:"replyText"`
	AudioFormat string `json:"audioFormat"` // "opus"
	SampleRate  int    `json:"sampleRate"`  // 24000
	Audio       []byte `json:"-"`           // decoded from base64
}
// RegisterDeviceResp 对齐 aisaas /devices/{id}/register 响应。
//
// 注意：aisaas 当前返回 tenantId/keyId 为 JSON 数字（不是字符串）。
// 本结构与 Persona/PersonaBind 的 ,string tag 不一致；如未来 aisaas 切换字符串型，
// 需同步加 ,string tag 并更新 aisaas 服务端。
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
	// aisaas 响应统一包装：{"code":0,"data":{...}}
	// resty SetResult 不会自动拆 data，需手动两段解析（与本文件 GetDevice 同模式）。
	var outer struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	resp, err := c.http.R().
		SetContext(ctx).
		SetHeader("X-Device-Id", deviceID).
		SetBody(body).
		SetResult(&outer).
		Post(c.baseURL + "/internal/api/v1/devices/" + deviceID + "/register")
	if err != nil {
		return nil, fmt.Errorf("register device: %w", err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, ErrDeviceNotRegistered
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("register device failed: status=%d body=%s", resp.StatusCode(), resp.String())
	}
	if outer.Code != 0 {
		return nil, fmt.Errorf("register device: code=%d data=%s", outer.Code, string(outer.Data))
	}
	var regResp RegisterDeviceResp
	if err := json.Unmarshal(outer.Data, &regResp); err != nil {
		return nil, fmt.Errorf("parse register device data: %w", err)
	}
	return &regResp, nil
}

// ErrPersonaNotBound 设备未绑定 persona；orchestrator 应回退到默认 prompt，不应让用户感知错误。
var ErrPersonaNotBound = fmt.Errorf("device has no persona bound")

// Dialogue POST /internal/api/v1/dialogue/run — single-call full-duplex dialogue
// that internally handles STT → LLM → TTS using the aisaas model registry.
// It replaces the old STT/Chat/TTS trio.
func (c *Client) Dialogue(ctx context.Context, deviceID string, wavBytes []byte) (*DialogueResult, error) {
	reqBody := map[string]interface{}{
		"deviceId":    deviceID,
		"audioFormat": "wav",
		"sampleRate":  16000,
		"audioBase64": base64.StdEncoding.EncodeToString(wavBytes),
	}
	resp, err := c.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(reqBody).
		Post(c.baseURL + "/internal/api/v1/dialogue/run")
	if err != nil {
		return nil, fmt.Errorf("dialogue request: %w", err)
	}

	// Handle non-OK status BEFORE attempting JSON parse of body
	// (aisaas may return plain text on 5xx)
	switch {
	case resp.StatusCode() >= 500:
		return nil, fmt.Errorf("dialogue internal error: status=%d body=%s", resp.StatusCode(), resp.String())
	case resp.StatusCode() == http.StatusNotFound:
		// Try to parse JSON for code/message; fall back to status-only check
		var errResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if e := json.Unmarshal(resp.Body(), &errResp); e == nil {
			if errResp.Code == 40404 || strings.Contains(errResp.Message, "device") {
				return nil, ErrDeviceNotRegistered
			}
			return nil, ErrModelNotFound
		}
		return nil, ErrDeviceNotRegistered
	case resp.StatusCode() == http.StatusTooManyRequests:
		return nil, ErrQuotaExceeded
	case resp.StatusCode() != http.StatusOK:
		return nil, fmt.Errorf("dialogue failed: status=%d body=%s", resp.StatusCode(), resp.String())
	}

	// 2xx — parse success wrapper
	var wrapper struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &wrapper); err != nil {
		return nil, fmt.Errorf("parse dialogue response: %w", err)
	}
	if wrapper.Code != 0 {
		return nil, fmt.Errorf("dialogue error: code=%d message=%s", wrapper.Code, wrapper.Message)
	}

	var data struct {
		UserText    string `json:"userText"`
		ReplyText   string `json:"replyText"`
		AudioFormat string `json:"audioFormat"`
		SampleRate  int    `json:"sampleRate"`
		AudioBase64 string `json:"audioBase64"`
	}
	if err := json.Unmarshal(wrapper.Data, &data); err != nil {
		return nil, fmt.Errorf("parse dialogue data: %w", err)
	}

	audioBytes, err := base64.StdEncoding.DecodeString(data.AudioBase64)
	if err != nil {
		return nil, fmt.Errorf("decode audio base64: %w", err)
	}

	return &DialogueResult{
		UserText:    data.UserText,
		ReplyText:   data.ReplyText,
		AudioFormat: data.AudioFormat,
		SampleRate:  data.SampleRate,
		Audio:       audioBytes,
	}, nil
}

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
	r := c.http.R().SetContext(ctx)
	c.applyAPIKey(r)
	resp, err := r.Get(url)
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
