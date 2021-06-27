package node

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
	"github.com/xvzf/vaa/pkg/neigh"
)

type Handler interface {
	Run(context.Context, chan *com.Message) error
}

// handler holds internal information & datastructures for a node
type handler struct {
	uid               uint
	neighs            *neigh.Neighs
	exit              context.CancelFunc
	wg                sync.WaitGroup
	rumor             *rumor
	consensusElection *consensusElection
	started           bool
}

// uidFromPayload parses the payload following `<some-extruction> <uid>`
func uidFromPayload(msg *com.Message) (uint, error) {
	ps := strings.Split(*msg.Payload, ";")
	if len(ps) != 2 {
		return 0, errors.New("payload is not of format <type><space><node-id>")
	}
	uid, err := strconv.Atoi(ps[1])
	if err != nil {
		return 0, err
	}
	return uint(uid), nil
}

func New(uid uint, exitFunc context.CancelFunc, neighs *neigh.Neighs) Handler {
	// Init datastructures of the node
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
		consensusElection: &consensusElection{
			echoFrom: make(map[uint]bool),
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

// handle routes incoming messages to the corresponding handlers
func (h *handler) handle(msg *com.Message) error {
	// Log incoming message (in addition to the dispatcher, as the dispatcher runs async and uses channels for interfacing with the node process)
	log.Info().
		Str("msg_direction", "incoming").
		Str("req_id", *msg.UUID).
		Uint("ttl", *msg.TTL).
		Time("timestamp", *msg.Timestamp).
		Uint("src_uid", *msg.SourceUID).
		Str("type", *msg.Type).
		Str("payload", *msg.Payload).
		Msg("<<<")

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
	case "CONSENSUS":
		return h.handleConsensus(msg)
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
		h.handleControl_startup(msg)
		return nil
	case strings.HasPrefix(payload, "DISTRIBUTE"): // Distribute messages
		log.Info().Uint("uid", h.uid).Str("req_id", *msg.UUID).Msgf("Disstribute messages")
		h.handleControl_distribute(msg)
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

	for nuid, netaddr := range h.neighs.Nodes {
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

// handleConsensus handles incoming consensus events
func (h *handler) handleConsensus(msg *com.Message) error {
	log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Handling consensus message")

	switch payload := *msg.Payload; {
	case strings.HasPrefix(payload, "explore"):
		h.handleConsensus_explore(msg)
	case strings.HasPrefix(payload, "echo"):
		h.handleConsensus_echo(msg)
	}

	return nil
}

// handleConsensus_explore contains leader-elect explore logic
func (h *handler) handleConsensus_explore(msg *com.Message) error {
	log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Handling consensus leader-elect explore")
	// Extract explore UID from payload
	euid, err := uidFromPayload(msg)
	if err != nil {
		log.Err(err).Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Invalid payload")
	}

	// Register explore; propagate if it is > M
	if h.consensusElection.M < int(euid) {
		log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).
			Msgf("First explore for %d, evicting %d", euid, h.consensusElection.M)
		h.consensusElection.M = int(euid)
		h.consensusElection.exploreFrom = make(map[uint]bool)
		h.consensusElection.exploreFrom[*msg.SourceUID] = true
		h.consensusElection.firstNUID = *msg.SourceUID

		// Propagate the explore
		pMsg := com.MsgPropagate(h.uid, msg)
		for nuid, netaddr := range h.neighs.Nodes {
			if nuid == *msg.SourceUID {
				// Don't send back explore
				continue
			}
			if err = com.Send(netaddr, pMsg); err != nil {
				log.Err(err).Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Failed to send echo")
			}
		}

	} else if h.consensusElection.M == int(euid) {
		// Consecutive receive
		h.consensusElection.exploreFrom[*msg.SourceUID] = true
	} else {
		// Ignoring explore
		log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).
			Msgf("Ignoring explore for %d, in favour of %d", euid, h.consensusElection.M)
		return nil
	}

	// Check if we should send an echo
	if diff := len(h.neighs.Nodes) - len(h.consensusElection.exploreFrom); diff > 0 {
		log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).
			Msgf("Missing explores for %d echo: %d", euid, diff)
	} else {
		log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).
			Msgf("Sending echo for %d", euid)
		// Construct echo message and send it
		eMsg := com.Msg(h.uid, "CONSENSUS", fmt.Sprintf("echo;%d", euid))
		if err = com.Send(h.neighs.Nodes[h.consensusElection.firstNUID], eMsg); err != nil {
			log.Err(err).Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Failed to send echo")
			return err
		}
	}

	return nil
}

// handleConsensus_echo performs the echo logic
func (h *handler) handleConsensus_echo(msg *com.Message) error {
	log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Handling consensus leader-elect echo")

	// Extract elected UID from payload
	euid, err := uidFromPayload(msg)
	if err != nil {
		log.Err(err).Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Invalid payload")
	}

	if euid < uint(h.consensusElection.M) {
		log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Evicting echo with uid %d", euid)
		return nil
	}

	// We're the winner?
	if euid == h.uid {
		h.consensusElection.echoFrom[*msg.SourceUID] = true
		log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).
			Msgf("Marked edge as echo received")

		// We won the election, launch consensusMain
		if len(h.consensusElection.echoFrom) == len(h.neighs.Nodes) && !h.consensusElection.wonElection {
			log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).
				Msgf("Election succeeded, I'm now the consensus leader")
			h.consensusElection.wonElection = true // don't start it multiple times
			go h.consensusMain(context.Background())
		}

		return nil
	}

	// If >= M forward to receiving edge
	h.consensusElection.M = int(euid)
	log.Info().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).
		Msgf("Sending echo with uid %d to node with uid", euid, h.consensusElection.firstNUID)
	rMsg := com.Msg(h.uid, *msg.Type, *msg.Payload) // copy message

	err = com.Send(h.neighs.Nodes[h.consensusElection.firstNUID], rMsg)
	if err != nil {
		log.Err(err).Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Failed propagating echo")
	}
	return nil
}

// consensusMain is launched on the node with successful leader election
func (h *handler) consensusMain(ctx context.Context) {
	log.Info().Msg("Starting Consensus leader")
	for {
		log.Info().Msg("Starting Consensus leader: alive")
		time.Sleep(1 * time.Second)
	}
}
