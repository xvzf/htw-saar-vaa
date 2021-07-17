// Leader election implementation based on explore/echo algorithm. It builds a distributed spanning tree
package node

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
)

type Leader struct {
	sync.Mutex

	messageType string
	wantLeader  bool
	isLeader    bool
	m           int
	leaderUID   uint // Will be > 0 when a leader has been selected; in this case all election messages are ignored

	// Those vars are linked to an active election and are resetted whenever m changes
	childUIDs         []uint // Childs of this node (spanning tree)
	receivedParentMsg int    // Used to track how many neighbours responded the explore
	receivedExplore   int    // Used to track how many explore messages have been received
	sentExplore       int    // Used to track how many explore messages have been sent
	receivedEcho      int    // Used to track how many echos have been received for
	srcUID            uint   // Parent of this node (if it has one, if it is leader -> its own UID)
}

func NewLeader(msgType string, wantLeader bool) *Leader {
	return &Leader{
		messageType: msgType,
		wantLeader:  wantLeader,
		isLeader:    false,
		m:           0,
		leaderUID:   0,

		childUIDs:         []uint{},
		receivedParentMsg: 0,
		receivedExplore:   0,
		sentExplore:       0,
		receivedEcho:      0,
		srcUID:            0,
	}
}

func (l *Leader) TryHandleLeaderMessage(h *handler, msg *com.Message) (bool, error) {
	switch payload := *msg.Payload; {
	case strings.HasPrefix(payload, "explore"): // Check if payload is allowed
		return true, l.handle_explore(h, msg)
	case strings.HasPrefix(payload, "child"): // Check if payload is allowed
		return true, l.handle_child(h, msg)
	case strings.HasPrefix(payload, "echo"): // Check if payload is allowed
		return true, l.handle_echo(h, msg)
	case strings.HasPrefix(payload, "coordinator"): // Check if payload is allowed
		return true, l.handle_coordinator(h, msg)
	case strings.HasPrefix(payload, "leader"): // Check if payload is allowed
		return true, l.handle_leader(h, msg)
	}
	return false, nil
}

func (l *Leader) ElectionComplete() bool {
	return l.leaderUID != 0
}

func (l *Leader) IsLeader() bool {
	return l.isLeader
}

// Propagates to all but sender
func (l *Leader) propagate(h *handler, msg *com.Message) int {
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

// propagateChilds propagates the leader election results across the child
func (l *Leader) propagateChilds(h *handler, msg *com.Message) int {
	total := 0
	for _, cuid := range l.childUIDs {
		err := com.Send(h.neighs.Nodes[cuid], com.MsgPropagate(h.uid, msg))
		if err != nil {
			log.Err(err).Msg("Failed to proagate")
		}
		total += 1
	}

	return total
}

// Checks if we should send an echo (either edge node or all echos from childs received)
func (l *Leader) checkSendEcho(h *handler) error {
	allParents := l.sentExplore == l.receivedParentMsg
	if !allParents {
		log.Info().Msgf("Not yet received all child messages %d/%d", l.receivedParentMsg, l.sentExplore)
		return nil
	}

	// - Received echos from all childs -> send echo or trigger leader
	// - No childs and received echo from all neighs -> send echo trigger leader
	if (len(l.childUIDs) == l.receivedEcho) || (len(l.childUIDs) == 0 && l.receivedExplore == len(h.neighs.Nodes)) {
		if l.m == int(h.uid) { // Check if this node was the initiator
			l.leaderUID = h.uid
			// Send election results
			log.Info().Msgf("Sending election result spanning tree (child nodes: %s)", l.childUIDs)
			l.propagateChilds(h, com.Msg(h.uid, l.messageType, fmt.Sprintf("leader;%d", h.uid)))
			// This node is now the leader! :)
			log.Info().Msgf("This node is now leader (%s)", l.messageType)
			l.isLeader = true
			return nil
		} else { // This node is not the leader, send echo alongside the spanning tree
			log.Info().Msgf("Send echo for %d to %d", l.m, l.srcUID)
			msg := com.Msg(h.uid, l.messageType, fmt.Sprintf("echo;%d", l.m))
			return com.Send(h.neighs.Nodes[l.srcUID], msg)
		}
	} else {
		log.Info().Msgf("Echo condition not met rec_exp=%d rec_echo=%d rec_prt=%d neigh=%d childUIDs=%d", l.receivedExplore, l.receivedEcho, l.receivedParentMsg, len(h.neighs.Nodes), len(l.childUIDs))
	}

	return nil
}

// Handle_coordinator starts the leader-election for the future vote coordinator
func (l *Leader) handle_coordinator(h *handler, msg *com.Message) error {
	if !l.wantLeader {
		log.Info().Uint("uid", h.uid).Msg("Not starting coordinator election")
		return nil
	}
	log.Info().Uint("uid", h.uid).Msg("Start coordinator election")
	// Set m to own
	l.m = int(h.uid)
	l.childUIDs = []uint{}
	l.receivedEcho = 0
	l.receivedExplore = 0
	l.srcUID = h.uid // own UID
	l.sentExplore = 0
	// Send explore to all neighbouirs
	for _, connect := range h.neighs.Nodes {
		err := com.Send(connect, com.Msg(h.uid, l.messageType, fmt.Sprintf("explore;%d", h.uid)))
		if err != nil {
			log.Err(err).Msg("Failed to send explore")
		}
		l.sentExplore += 1
	}
	return nil
}

// Handle_leader sets the leader status of the network
func (l *Leader) handle_leader(h *handler, msg *com.Message) error {
	luid, err := nthInt(*msg.Payload, 1)
	if err != nil {
		return err
	}

	log.Info().Uint("uid", h.uid).Msgf("Setting leaderUID to %d", luid)

	// Set m to own
	l.leaderUID = uint(luid)

	log.Info().Msgf("Propagating leader message to %s", l.childUIDs)
	l.propagateChilds(h, msg)
	return nil
}

// Handle_explore handles incoming explore messages
func (l *Leader) handle_explore(h *handler, msg *com.Message) error {
	if l.leaderUID != 0 {
		log.Warn().Str("req_id", *msg.UUID).Msgf("%d is already the leader, ignoring", l.leaderUID)
		return nil
	}

	euid, err := nthInt(*msg.Payload, 1)
	if err != nil {
		return err
	}

	if euid > l.m { // Larger m received
		log.Info().Msgf("Explore %d > current %d, evicting", euid, l.m)
		// Initalize internal datastructure
		l.m = euid
		l.receivedParentMsg = 0
		l.receivedExplore = 1
		l.sentExplore = 0
		l.childUIDs = []uint{}
		l.srcUID = *msg.SourceUID

		// Send child message to parent
		com.Send(h.neighs.Nodes[l.srcUID], com.Msg(h.uid, l.messageType, fmt.Sprintf("child;%d;1", l.m)))

		// Propagate to neighs
		l.sentExplore = l.propagate(h, msg)

	} else if euid == l.m { // Already known; not child
		com.Send(h.neighs.Nodes[*msg.SourceUID], com.Msg(h.uid, l.messageType, fmt.Sprintf("child;%d;0", l.m)))
		l.receivedExplore += 1
	} else { // Lower m received; evicted
		log.Info().Msgf("Evicted EXPLORE %d in favour of %d", euid, l.m)
		return nil
	}

	// Check if edge node; trigger echo
	return l.checkSendEcho(h)
}

// Handle_explore handles incoming child messages
func (l *Leader) handle_child(h *handler, msg *com.Message) error {
	if l.leaderUID != 0 {
		log.Warn().Str("req_id", *msg.UUID).Msgf("%d is already the leader, ignoring", l.leaderUID)
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

	if euid > l.m {
		log.Error().Msg("Invalid state, received child for UID > current m -> not send by this node")
		return errors.New("invalid state")
	} else if euid == l.m {
		l.receivedParentMsg += 1
		// Add child to child list (-> distributed spanning tree)
		if child == 1 {
			l.childUIDs = append(l.childUIDs, *msg.SourceUID)
		}
		log.Info().Msgf("Increase isParent received to %d, total child count: %d", l.receivedParentMsg, len(l.childUIDs))
		// Check if edge node; trigger echo
		l.checkSendEcho(h)
	} else {
		log.Info().Msgf("Ignore child for %d, voting for", euid, l.m)
	}

	return nil
}

// Handle_explore handles incoming echo messages
func (l *Leader) handle_echo(h *handler, msg *com.Message) error {
	if l.leaderUID != 0 {
		log.Warn().Str("req_id", *msg.UUID).Msgf("%d is already the leader, ignoring", l.leaderUID)
		return nil
	}
	euid, err := nthInt(*msg.Payload, 1)
	if err != nil {
		return err
	}

	if euid > l.m {
		log.Error().Msgf("Invalid state: %d > current %d, not triggered by this node", euid, l.m)
		return errors.New("invalid state")
	} else if euid == l.m {
		// Increase received echo counter; check if we should propagate
		l.receivedEcho += 1
		// log.Warn().Msg("We should not be here, child messages should always come in before echo; processing nevertheless")
		l.checkSendEcho(h) // FIXME should not happen
	} else {
		log.Info().Msgf("Evicted ECHO %d in favour of %d", euid, l.m)
	}

	return nil
}
