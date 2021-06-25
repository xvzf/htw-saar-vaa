package node

import (
	"context"
	"errors"
	"strconv"
	"strings"
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
	uid     uint
	neighs  *neigh.Neighs
	exit    context.CancelFunc
	wg      sync.WaitGroup
	rumor   *rumor
	started bool
}

func New(uid uint, exitFunc context.CancelFunc, neighs *neigh.Neighs) Handler {
	return &handler{
		uid:     uid,
		exit:    exitFunc,
		wg:      sync.WaitGroup{},
		neighs:  neighs,
		started: false,
		rumor: &rumor{
			counter: make(map[string]int),
			trusted: make(map[string]bool),
		},
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
	case "RUMOR":
		return h.handleRumor(msg)
	}
	log.Warn().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Message type `%s` not supported", *msg.Type)
	return nil
}

// handleControl handles control messages for starting/stopping the node
func (h *handler) handleControl(msg *com.Message) error {
	// Control message; we do not log the src_uid here since it is irrelevant for the control protocol
	log.Info().Uint("uid", h.uid).Str("req_id", *msg.UUID).Msgf("Handling control message")

	switch payload := *msg.Payload; {
	// Graceful shutdown of the cluster
	case payload == "SHUTDOWN": // Shutdown message, directly handled here
		log.Info().Uint("uid", h.uid).Str("req_id", *msg.UUID).Msgf("Initiated node shutdown")
		h.exit()
		return nil
	// Startup
	case payload == "STARTUP": // Startup messages
		log.Info().Uint("uid", h.uid).Str("req_id", *msg.UUID).Msgf("Initiated node startup")
		go h.handleControl_startup(msg)
		return nil
	case strings.HasPrefix(payload, "DISTRIBUTE"): // Distribute messages
		log.Info().Uint("uid", h.uid).Str("req_id", *msg.UUID).Msgf("Disstribute messages")
		go h.handleControl_distribute(msg)
		return nil
	}

	return nil
}

func (h *handler) handleControl_startup(msg *com.Message) error {
	helloMsg := com.Msg(h.uid, "DISCOVERY", "HELLO")
	if h.started {
		log.Warn().Uint("uid", h.uid).Msg("Node already started")
		return nil
	}
	h.started = true

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

func (h *handler) handleControl_distribute(msg *com.Message) error {

	ps := strings.Split(*msg.Payload, " ")
	if len(ps) != 3 {
		err := errors.New("payload invalid")
		log.Err(err).Msg("Failed to distribute")
	}
	t, p := ps[1], ps[2]

	toSend := com.Msg(h.uid, t, p)

	// Send HELLO to all neighbors
	for nuid, netaddr := range h.neighs.Nodes {
		log.Info().Uint("uid", h.uid).Msgf("Sending %s to %d", t, nuid)
		if err := com.Send(netaddr, toSend); err != nil {
			log.Err(err).Uint("uid", h.uid).Msgf("Failed sending %s to %d", t, nuid)
		}
	}

	return nil
}

// handleDiscover_hello handles hello messages and marks neighbors as active
func (h *handler) handleDiscovery_hello(msg *com.Message) error {
	// Mark neighbour as registered
	h.neighs.Registered[*msg.SourceUID] = true
	log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).
		Msgf("Registered node with UID %d", *msg.SourceUID)
	return nil
}

// handleRumor handles incoming rumor events
func (h *handler) handleRumor(msg *com.Message) error {
	log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Handling rumor message")
	ps := strings.Split(*msg.Payload, ";")
	if len(ps) != 2 {
		log.Error().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Invalid message payload")
		return errors.New("invalid message payload")
	}

	// Extract payload
	r := ps[1]
	c, err := strconv.Atoi(ps[0])
	if err != nil {
		log.Err(err).Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("C invalid")
		return err
	}

	// Increase rumor counter
	s := h.rumor.Add(r)

	// Seen this rumor the first time -> distribute
	if s == 1 {
		msgProgatate := com.MsgPropagate(h.uid, msg)
		for nuid, netaddr := range h.neighs.Nodes {
			if nuid == *msg.SourceUID {
				// Skip sending the event to receiving edge
				continue
			}

			// Propagate to neighbor
			log.Info().Uint("uid", h.uid).Msgf("Propagating rumor `%s` to %d", *msgProgatate.Payload, nuid)
			if err := com.Send(netaddr, msgProgatate); err != nil {
				log.Err(err).Uint("uid", h.uid).Msgf("Failed Rumor %s to %d", *msgProgatate.Payload, nuid)
			}
		}
	}

	log.Info().Uint("uid", h.uid).Str("rumor", r).Int("seen", s).
		Msgf("Counter increased")

	if s == c { // Initially trusted
		h.rumor.Trusted(r)
		log.Info().Uint("uid", h.uid).Str("rumor", r).Int("seen", s).
			Msgf("Now trusted")
	} else if s > c { // Already trusted
		log.Info().Uint("uid", h.uid).Str("rumor", r).Int("seen", s).
			Msgf("Trusted since %d shares", s-c)
	}
	return nil
}
