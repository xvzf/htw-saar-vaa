package node

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
)

type Handler interface {
	Run(context.Context, chan *com.Message) error
}

// handler holds internal information & datastructures for a node
type handler struct {
	UID  uint
	exit context.CancelFunc
	wg   sync.WaitGroup
}

func New(uid uint, exitFunc context.CancelFunc) Handler {
	return &handler{
		UID:  uid,
		exit: exitFunc,
		wg:   sync.WaitGroup{},
	}
}

func (h *handler) Run(ctx context.Context, c chan *com.Message) error {
	// Receive until context exits
	log.Info().Uint("uid", h.UID).Msg("Starting node")
	for {
		select {
		case msg := <-c:
			if err := h.handle(msg); err != nil {
				log.Err(err).Str("req_id", *msg.UUID).Msg("Failed handling incoming message")
			}
		case <-ctx.Done():
			h.wg.Wait()
			log.Info().Uint("uid", h.UID).Msg("Node shutdown complete")
			return nil
		}
	}
}

// handle distributes incoming messages
func (h *handler) handle(msg *com.Message) error {
	// Mark processing start/end; this allows us to cracefully shutdown on context cancelation
	h.wg.Add(1)
	defer h.wg.Done()

	// Handle message
	log.Debug().Uint("uid", h.UID).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msg("Distributing")
	switch *msg.Type {
	case "CONTROL":
		return h.handleControl(msg)
	}
	log.Warn().Uint("uid", h.UID).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Message type `%s` not supported", *msg.Type)
	return nil
}

// handleControl handles control messages for starting/stopping the node
func (h *handler) handleControl(msg *com.Message) error {
	// Control message; we do not log the src_uid here since it is irrelevant for the control protocol
	log.Info().Uint("uid", h.UID).Str("req_id", *msg.UUID).Msgf("Handling control message")

	switch *msg.Payload {
	// Graceful shutdown of the cluster
	case "SHUTDOWN":
		log.Info().Uint("uid", h.UID).Str("req_id", *msg.UUID).Msgf("Initiated node shutdown")
		h.exit()
		return nil
	// Startup
	case "STARTUP":
		log.Info().Uint("uid", h.UID).Str("req_id", *msg.UUID).Msgf("Initiated node startup, not implemented")
		return nil
	}

	return nil
}
