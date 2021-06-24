package node

import (
	"context"

	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
)

type discovery struct{}

func NewDiscoveryExtension() (Extension, string) {
	return &discovery{}, "DISCOVERY"
}
func (d *discovery) Preflight(ctx context.Context, h *handler) error {
	// not required
	return nil
}

func (d *discovery) Handle(h *handler, msg *com.Message) error {
	// Mark neighbour as registered
	h.neighs.Registered[*msg.SourceUID] = true
	log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).
		Msgf("Registered node with UID %d", *msg.SourceUID)
	return nil
}
