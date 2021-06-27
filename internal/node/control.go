package node

import (
	"errors"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
)

type control struct {
	started bool
}

func NewControlExtension() (Extension, string) {
	return &control{started: false}, "CONTROL"
}

func (c *control) Handle(h *handler, msg *com.Message) error {
	// Control message; we do not log the src_uid here since it is irrelevant for the control protocol
	log.Debug().Uint("uid", h.uid).Str("req_id", *msg.UUID).Msgf("Handling control message")

	switch payload := *msg.Payload; {
	// Graceful shutdown of the cluster
	case payload == "SHUTDOWN": // Shutdown message, directly handled here
		log.Info().Uint("uid", h.uid).Str("req_id", *msg.UUID).Msgf("Initiated node shutdown")
		h.exit()
		return nil
	// Startup
	case payload == "STARTUP": // Startup messages
		c.handleControl_startup(h, msg)
		return nil
	case strings.HasPrefix(payload, "DISTRIBUTE"): // Distribute messages
		log.Debug().Uint("uid", h.uid).Str("req_id", *msg.UUID).Msgf("Disstribute messages")
		c.handleControl_distribute(h, msg)
		return nil
	}

	return nil
}

func (c *control) handleControl_startup(h *handler, msg *com.Message) error {
	helloMsg := com.Msg(h.uid, "DISCOVERY", "HELLO")
	if c.started {
		log.Warn().Uint("uid", h.uid).Msg("Node already started")
		return nil
	}
	c.started = true

	// Send HELLO to all neighbors
	for nuid, netaddr := range h.neighs.Nodes {
		log.Debug().Uint("uid", h.uid).Msgf("Sending HELLO to %d", nuid)
		if err := com.Send(netaddr, helloMsg); err != nil {
			log.Err(err).Uint("uid", h.uid).Msgf("Failed sending HELLO to %d", nuid)
		}
	}

	return nil
}

func (c *control) handleControl_distribute(h *handler, msg *com.Message) error {

	ps := strings.Split(*msg.Payload, " ")
	if len(ps) != 3 {
		err := errors.New("payload invalid")
		log.Err(err).Msg("Failed to distribute")
	}
	t, p := ps[1], ps[2]

	toSend := com.Msg(h.uid, t, p)

	for nuid, netaddr := range h.neighs.Nodes {
		if err := com.Send(netaddr, toSend); err != nil {
			log.Err(err).Uint("uid", h.uid).Msgf("Failed sending %s to %d", t, nuid)
		}
	}

	return nil
}
