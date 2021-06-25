package node

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
	"github.com/xvzf/vaa/pkg/neigh"
)

type Handler interface {
	Run(context.Context, chan *com.Message) error
}

// handler holds internal information & datastructures for a node
type handler struct {
	uid    uint
	neighs *neigh.Neighs
	exit   context.CancelFunc
	wg     sync.WaitGroup
}

func New(uid uint, exitFunc context.CancelFunc, neighs *neigh.Neighs) Handler {
	return &handler{
		uid:    uid,
		exit:   exitFunc,
		wg:     sync.WaitGroup{},
		neighs: neighs,
	}
}

func (h *handler) Run(ctx context.Context, c chan *com.Message) error {
	// Receive until context exits
	log.Info().Uint("uid", h.uid).Msg("Starting node")
	log.Info().Uint("uid", h.uid).Msgf("Registered neighbors: %s", h.neighs.Nodes)
	for {
		select {
		case msg := <-c:
			if err := h.handle(msg); err != nil {
				log.Err(err).Str("req_id", *msg.UUID).Msg("Failed handling incoming message")
			}
		case <-ctx.Done():
			h.wg.Wait()
			log.Info().Uint("uid", h.uid).Msg("Node shutdown complete")
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
	log.Debug().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msg("Distributing")
	switch *msg.Type {
	case "CONTROL":
		return h.handleControl(msg)
	case "DISCOVERY":
		return h.handleDiscovery(msg)
	}
	log.Warn().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Message type `%s` not supported", *msg.Type)
	return nil
}

// handleControl handles control messages for starting/stopping the node
func (h *handler) handleControl(msg *com.Message) error {
	// Control message; we do not log the src_uid here since it is irrelevant for the control protocol
	log.Info().Uint("uid", h.uid).Str("req_id", *msg.UUID).Msgf("Handling control message")

	switch *msg.Payload {
	// Graceful shutdown of the cluster
	case "SHUTDOWN":
		log.Info().Uint("uid", h.uid).Str("req_id", *msg.UUID).Msgf("Initiated node shutdown")
		h.exit()
		return nil
	// Startup
	case "STARTUP":
		log.Info().Uint("uid", h.uid).Str("req_id", *msg.UUID).Msgf("Initiated node startup")
		go h.handleControl_startup(msg)
		return nil
	}

	return nil
}

func (h *handler) handleControl_startup(msg *com.Message) error {
	helloMsg := com.Msg(h.uid, "DISCOVERY", "HELLO")

	// Send HELLO to all neighbors
	for nuid, netaddr := range h.neighs.Nodes {
		log.Info().Uint("uid", h.uid).Msgf("Sending HELLO to %d", nuid)
		if err := com.Send(netaddr, helloMsg); err != nil {
			log.Err(err).Uint("uid", h.uid).Msgf("Failed sending HELLO to %d", nuid)
		}
	}

	return nil
}

// handleDiscovery handles control messages for starting/stopping the node
func (h *handler) handleDiscovery(msg *com.Message) error {
	log.Info().Uint("uid", h.uid).Uint("uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Handling discovery message")

	switch *msg.Payload {
	// Node registration
	case "HELLO":
		h.handleDiscovery_hello(msg)
		return nil
	}

	return nil
}

func (h *handler) handleDiscovery_hello(msg *com.Message) error {
	// Mark neighbour as registered
	h.neighs.Registered[*msg.SourceUID] = true
	log.Info().Uint("uid", h.uid).Uint("uid", *msg.SourceUID).Str("req_id", *msg.UUID).
		Msgf("Registered node with UID %d", *msg.SourceUID)
	return nil
}
