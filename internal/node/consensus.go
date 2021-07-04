package node

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
)

type consensusState struct {
	active        bool
	msgInCounter  int
	msgOutCounter int
}

type consensus struct {
	// Vars used for leader election
	m          int
	wantLeader bool // Random init at startup time
	leaderUID  uint // Will be > 0 when a leader has been selected; in this case all election messages are ignored

	// Those vars are linked to an active election and are resetted whenever m changes
	childUIDs         []uint // Childs of this node (spanning tree)
	receivedParentMsg int    // Used to track how many neighbours responded the explore
	receivedExplore   int    // Used to track how many explore messages have been received
	sentExplore       int    // Used to track how many explore messages have been sent
	receivedEcho      int    // Used to track how many echos have been received for
	srcUID            uint   // Parent of this node (if it has one, if it is leader -> its own UID)

	// Echo communication for state/collect requests.
	echo map[string]int // map[<req_uid>]<counter_recv_messages>

	// State
	state    *consensusState            // This node state
	accState map[string]*consensusState // Accumulated state for state requests
}

func NewConsensusExtension() (Extension, string) {
	rand.Seed(time.Now().UnixNano())
	wantLeader := rand.Intn(2) == 1 // 50% chance of being true
	log.Info().Msgf("Wants to be leader: %t", wantLeader)
	return &consensus{
		// Consensus related
		m:                 0,
		childUIDs:         make([]uint, 0),
		receivedParentMsg: 0,
		receivedExplore:   0,
		sentExplore:       0,
		receivedEcho:      0,
		leaderUID:         0,
		srcUID:            0,
		wantLeader:        wantLeader,

		// Echo communication
		echo: map[string]int{},

		// State
		state: &consensusState{
			active:        false,
			msgInCounter:  0,
			msgOutCounter: 0,
		},
		accState: make(map[string]*consensusState),
	}, "CONSENSUS"
}

func (c *consensus) Handle(h *handler, msg *com.Message) error {
	switch payload := *msg.Payload; {
	case strings.HasPrefix(payload, "explore"): // Check if payload is allowed
		return c.handle_explore(h, msg)
	case strings.HasPrefix(payload, "child"): // Check if payload is allowed
		return c.handle_child(h, msg)
	case strings.HasPrefix(payload, "echo"): // Check if payload is allowed
		return c.handle_echo(h, msg)
	case strings.HasPrefix(payload, "coordinator"): // Check if payload is allowed
		return c.handle_coordinator(h, msg)
	case strings.HasPrefix(payload, "leader"): // Check if payload is allowed
		return c.handle_leader(h, msg)
	// State Request needs to be checked before the response
	case strings.HasPrefix(payload, "stateRequest"): // Check if payload is allowed
		return c.handle_stateRequest(h, msg)
	case strings.HasPrefix(payload, "stateResponse"): // Check if payload is allowed
		return c.handle_stateResponse(h, msg)
	}
	log.Warn().Msg("Payload not supported")
	return nil
}

// leader is the control method
func (c *consensus) leader(h *handler) error {
	log.Warn().Msg("this node is now leader; not implemented")
	// Propagate election result at spanning tree
	c.leaderUID = h.uid
	log.Info().Msgf("Sending election result spanning tree (child nodes: %s)", c.childUIDs)
	c.propagateChilds(h, com.Msg(h.uid, "CONSENSUS", fmt.Sprintf("leader;%d", h.uid)))

	// Select up to S random philosophs (max number of neigh)

	// Double counting
	log.Info().Msg("Start state request")
	c.echo["test1234"] = 0
	c.accState["test1234"] = &consensusState{false, 0, 0}
	m := com.Msg(h.uid, "CONSENSUS", "stateRequest;test1234")
	_ = c.propagateChilds(h, m)

	return nil
}

// Checks if we should send an echo (either edge node or all echos from childs received)
func (c *consensus) checkSendEcho(h *handler) error {
	allParents := c.sentExplore == c.receivedParentMsg
	if !allParents {
		log.Info().Msgf("Not yet received all child messages %d/%d", c.receivedParentMsg, c.sentExplore)
		return nil
	}

	// - Received echos from all childs -> send echo or trigger leader
	// - No childs and received echo from all neighs -> send echo trigger leader
	if (len(c.childUIDs) == c.receivedEcho) || (len(c.childUIDs) == 0 && c.receivedExplore == len(h.neighs.Nodes)) {
		if c.m == int(h.uid) { // Check if this node was the initiator
			return c.leader(h)
		} else { // This node is not the leader, send echo alongside the spanning tree
			log.Info().Msgf("Send echo for %d to %d", c.m, c.srcUID)
			msg := com.Msg(h.uid, "CONSENSUS", fmt.Sprintf("echo;%d", c.m))
			return com.Send(h.neighs.Nodes[c.srcUID], msg)
		}
	} else {
		log.Warn().Msgf("Echo condition not met rec_exp=%d rec_echo=%d rec_prt=%d neigh=%d childUIDs=%d", c.receivedExplore, c.receivedEcho, c.receivedParentMsg, len(h.neighs.Nodes), len(c.childUIDs))
	}

	return nil
}

// Propagates to all but sender
func (c *consensus) propagate(h *handler, msg *com.Message) int {
	total := 0
	for nuid, connect := range h.neighs.Nodes {
		if nuid == *msg.SourceUID {
			continue // skip sending to receiver
		}
		err := com.Send(connect, com.MsgPropagate(h.uid, msg))
		if err != nil {
			log.Err(err).Msg("Failed to proagate")
		}
		total += 1
	}

	return total
}

// Propagates to all childs (-> along the spanning tree)
func (c *consensus) propagateChilds(h *handler, msg *com.Message) int {
	total := 0
	for _, cuid := range c.childUIDs {
		err := com.Send(h.neighs.Nodes[cuid], com.MsgPropagate(h.uid, msg))
		if err != nil {
			log.Err(err).Msg("Failed to proagate")
		}
		total += 1
	}

	return total
}

func (c *consensus) stateReturn(h *handler, sUID string) {
	// Retrieve current state
	stateReceivedCount, ok := c.echo[sUID]
	if !ok {
		log.Error().Msgf("stateRequest %s does not exist", sUID)
	}
	accState, ok := c.accState[sUID]
	if !ok || accState == nil {
		log.Error().Msgf("stateRequest %s does not exist or accState not initialized", sUID)
	}

	// Forward echo
	if stateReceivedCount == len(c.childUIDs) {
		// Add this node state to accState
		accState.active = accState.active || c.state.active
		accState.msgInCounter = accState.msgInCounter + c.state.msgInCounter
		accState.msgOutCounter = accState.msgOutCounter + c.state.msgOutCounter

		// Construct message & send it
		if c.leaderUID == h.uid {
			log.Warn().Msgf("Received state; not implemented (%s, %t, %d, %d)", sUID, accState.active, accState.msgInCounter, accState.msgOutCounter)
		} else {
			sMsg := com.Msg(h.uid, "CONSENSUS", fmt.Sprintf("stateResponse;%s;%t;%d;%d", sUID, accState.active, accState.msgInCounter, accState.msgOutCounter))
			log.Info().Msgf("Propagate (accumulated) state to %d", c.srcUID)
			com.Send(h.neighs.Nodes[c.srcUID], sMsg)
		}

		/*
			// Cleanup the maps (prevent leakage)
			delete(c.echo, sUID)
			delete(c.accState, sUID)
		*/
	}

}

// Handle_state is used for identifying via the double counting method, if all nodes proceeded
func (c *consensus) handle_stateRequest(h *handler, msg *com.Message) error {
	// This message requires a leader (-> initialised spanning tree) to be present
	if c.leaderUID == 0 {
		return errors.New("no leader in network")
	}
	sUID, err := nthString(*msg.Payload, 1)
	if err != nil {
		return err
	}

	// Init response counter (if not existing yet); then propagate to childs
	if _, ok := c.echo[sUID]; ok {
		return fmt.Errorf("state request with uid %s already exists", sUID)
	}
	// Initialize echo-based state collection
	c.echo[sUID] = 0
	c.accState[sUID] = &consensusState{
		active:        false,
		msgInCounter:  0,
		msgOutCounter: 0,
	}

	// Propagate to all childs
	_ = c.propagateChilds(h, msg)

	// Check if we should return state early (-> when leaf node)
	c.stateReturn(h, sUID)
	return nil
}

// Handle_state is used for identifying via the double counting method, if all nodes proceeded
func (c *consensus) handle_stateResponse(h *handler, msg *com.Message) error {
	// This message requires a leader (-> initialised spanning tree) to be present
	if c.leaderUID == 0 {
		return errors.New("no leader in network")
	}
	sUID, err := nthString(*msg.Payload, 1)
	if err != nil {
		return err
	}
	isActive, err := nthBool(*msg.Payload, 2)
	if err != nil {
		return err
	}
	msgIn, err := nthInt(*msg.Payload, 3)
	if err != nil {
		return err
	}
	msgOut, err := nthInt(*msg.Payload, 4)
	if err != nil {
		return err
	}
	eCount, ok := c.echo[sUID]
	if !ok {
		return fmt.Errorf("state request with uid %s does not exists", sUID)
	}
	accState, ok := c.accState[sUID]
	if !ok || accState == nil {
		return fmt.Errorf("state request with uid %s does not exists or accState not initialized", sUID)
	}

	// Increase echo counter
	c.echo[sUID] = eCount + 1
	accState.active = accState.active || isActive
	accState.msgInCounter = accState.msgInCounter + msgIn
	accState.msgOutCounter = accState.msgOutCounter + msgOut

	// check if we should return state
	c.stateReturn(h, sUID)

	return nil
}

// Handle_coordinator starts the leader-election for the future vote coordinator
func (c *consensus) handle_coordinator(h *handler, msg *com.Message) error {
	if !c.wantLeader {
		log.Info().Uint("uid", h.uid).Msg("Not starting coordinator election")
		return nil
	}
	log.Info().Uint("uid", h.uid).Msg("Start coordinator election")
	// Set m to own
	c.m = int(h.uid)
	c.childUIDs = []uint{}
	c.receivedEcho = 0
	c.receivedExplore = 0
	c.srcUID = h.uid // own UID
	c.sentExplore = 0
	// Send explore to all neighbouirs
	for _, connect := range h.neighs.Nodes {
		err := com.Send(connect, com.Msg(h.uid, "CONSENSUS", fmt.Sprintf("explore;%d", h.uid)))
		if err != nil {
			log.Err(err).Msg("Failed to send explore")
		}
		c.sentExplore += 1
	}
	return nil
}

// Handle_leader sets the leader status of the network
func (c *consensus) handle_leader(h *handler, msg *com.Message) error {
	luid, err := nthInt(*msg.Payload, 1)
	if err != nil {
		return err
	}

	log.Info().Uint("uid", h.uid).Msgf("Setting leaderUID to %d", luid)

	// Set m to own
	c.leaderUID = uint(luid)

	log.Info().Msgf("Propagating leader message to %s", c.childUIDs)
	c.propagateChilds(h, msg)
	return nil
}

// Handle_explore handles incoming explore messages
func (c *consensus) handle_explore(h *handler, msg *com.Message) error {
	if c.leaderUID != 0 {
		log.Warn().Str("req_id", *msg.UUID).Msgf("%d is already the leader, ignoring", c.leaderUID)
		return nil
	}

	euid, err := nthInt(*msg.Payload, 1)
	if err != nil {
		return err
	}

	if euid > c.m { // Larger m received
		log.Info().Msgf("Explore %d > current %d, evicting", euid, c.m)
		// Initalize internal datastructure
		c.m = euid
		c.receivedParentMsg = 0
		c.receivedExplore = 1
		c.sentExplore = 0
		c.childUIDs = []uint{}
		c.srcUID = *msg.SourceUID

		// Send child message to parent
		com.Send(h.neighs.Nodes[c.srcUID], com.Msg(h.uid, "CONSENSUS", fmt.Sprintf("child;%d;1", c.m)))

		// Propagate to neighs
		c.sentExplore = c.propagate(h, msg)

	} else if euid == c.m { // Already known; not child
		com.Send(h.neighs.Nodes[*msg.SourceUID], com.Msg(h.uid, "CONSENSUS", fmt.Sprintf("child;%d;0", c.m)))
		c.receivedExplore += 1
	} else { // Lower m received; evicted
		log.Info().Msgf("Evicted EXPLORE %d in favour of %d", euid, c.m)
		return nil
	}

	// Check if edge node; trigger echo
	return c.checkSendEcho(h)
}

// Handle_explore handles incoming child messages
func (c *consensus) handle_child(h *handler, msg *com.Message) error {
	if c.leaderUID != 0 {
		log.Warn().Str("req_id", *msg.UUID).Msgf("%d is already the leader, ignoring", c.leaderUID)
		return nil
	}
	euid, err := nthInt(*msg.Payload, 1)
	if err != nil {
		return err
	}

	child, err := nthInt(*msg.Payload, 2)
	if err != nil {
		return err
	}

	if euid > c.m {
		log.Error().Msg("Invalid state, received child for UID > current m -> not send by this node")
		return errors.New("invalid state")
	} else if euid == c.m {
		c.receivedParentMsg += 1
		// Add child to child list (-> distributed spanning tree)
		if child == 1 {
			c.childUIDs = append(c.childUIDs, *msg.SourceUID)
		}
		log.Info().Msgf("Increase isParent received to %d, total child count: %d", c.receivedParentMsg, len(c.childUIDs))
		// Check if edge node; trigger echo
		c.checkSendEcho(h)
	} else {
		log.Info().Msgf("Ignore child for %d, voting for", euid, c.m)
	}

	return nil
}

// Handle_explore handles incoming echo messages
func (c *consensus) handle_echo(h *handler, msg *com.Message) error {
	if c.leaderUID != 0 {
		log.Warn().Str("req_id", *msg.UUID).Msgf("%d is already the leader, ignoring", c.leaderUID)
		return nil
	}
	euid, err := nthInt(*msg.Payload, 1)
	if err != nil {
		return err
	}

	if euid > c.m {
		log.Error().Msgf("Invalid state: %d > current %d, not triggered by this node", euid, c.m)
		return errors.New("invalid state")
	} else if euid == c.m {
		// Increase received echo counter; check if we should propagate
		c.receivedEcho += 1
		// log.Warn().Msg("We should not be here, child messages should always come in before echo; processing nevertheless")
		c.checkSendEcho(h) // FIXME should not happen
	} else {
		log.Info().Msgf("Evicted ECHO %d in favour of %d", euid, c.m)
	}

	return nil
}
