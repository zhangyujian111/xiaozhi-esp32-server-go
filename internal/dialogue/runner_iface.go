package dialogue

import (
	"context"

	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
)

// OrchestratorRunner is the minimal contract ws.Handler depends on to drive a dialogue turn.
// Defined here to avoid a circular import (dialogue → ws already exists; we add the inverse contract).
type OrchestratorRunner interface {
	Run(ctx context.Context, sessionID string, conn Conn, device *aisaas.DeviceInfo, audioBytes []byte) error
}