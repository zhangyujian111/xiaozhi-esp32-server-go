package aisaas

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

type Client struct {
	http          *resty.Client
	baseURL      string
	internalToken string
}

func NewClient(baseURL, internalToken string) *Client {
	c := resty.New().
		SetTimeout(30 * time.Second).
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
	var info DeviceInfo
	resp, err := c.http.R().
		SetContext(ctx).
		SetHeader("X-Device-Id", deviceID).
		SetResult(&info).
		Get(c.baseURL + "/internal/api/v1/devices/" + deviceID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get device failed: status=%d body=%s", resp.StatusCode(), resp.String())
	}
	return &info, nil
}

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
