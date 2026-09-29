package aisaas

import (
	"context"
	"errors"
)

var ErrNotImplemented = errors.New("not implemented yet")

type Client struct {
	BaseURL       string
	InternalToken string
}

func (c *Client) VerifyDeviceToken(ctx context.Context, deviceID, token string) error {
	return ErrNotImplemented
}

type Authenticator interface {
	VerifyDeviceToken(ctx context.Context, deviceID, token string) error
}
