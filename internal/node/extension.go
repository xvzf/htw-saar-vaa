package node

import (
	"context"

	"github.com/xvzf/vaa/pkg/com"
)

type Extension interface {
	// Hanlde handles messages of a specific type
	Handle(h *handler, msg *com.Message) error
	// Preflight initialised additional goroutines/communication paths
	Preflight(ctx context.Context, h *handler) error
}
