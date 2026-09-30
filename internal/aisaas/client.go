package aisaas

import (
	"context"
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
}

func (c *Client) GetDevice(ctx context.Context, deviceID string) (*DeviceInfo, error) {
	var bindResp struct {
		DeviceID string `json:"deviceId"`
		TenantID int64  `json:"tenantId"`
		BindCode string `json:"bindCode"`
	}
	resp, err := c.http.R().
		SetContext(ctx).
		SetHeader("X-Device-Id", deviceID).
		SetResult(&bindResp).
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
	return &DeviceInfo{DeviceID: bindResp.DeviceID, UserID: bindResp.TenantID}, nil
}

var ErrDeviceNotRegistered = fmt.Errorf("device not registered with aisaas")

func (c *Client) VerifyDeviceToken(ctx context.Context, deviceID, token string) error {
	if token == "" {
		return fmt.Errorf("empty token")
	}
	_, err := c.GetDevice(ctx, deviceID)
	return err
}

type Authenticator interface {
	VerifyDeviceToken(ctx context.Context, deviceID, token string) error
}

var _ Authenticator = (*Client)(nil)
