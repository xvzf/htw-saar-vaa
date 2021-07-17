package node

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
)

// Distributed Banking
type banking struct {
	// Vars used for leader election
	leader *Leader

	// Lamport clock
	lc *lamportClock

	// Lamport mutual exclusion
	lm                *lamportMutexQueue
	lockRequestLC     int
	lockRequestActive bool
	lockAckCounter    int

	// Flooding
	known map[string]struct{} // keep track of known mesages

	// Tranaction balance
	balance                 int
	randP                   int
	transactAckReceived     bool
	transactBalanceReceived bool
}

func NewDistributedBankingExtension() (Extension, string) {
	rand.Seed(time.Now().UnixNano())
	wantLeader := rand.Intn(2) == 1 // 50% chance of being true
	log.Info().Msgf("Wants to be leader: %t", wantLeader)
	return &banking{
		// Leader Election / communicate to leader
		leader: NewLeader("BANKING", wantLeader),

		// Lamport Clock
		lc: &lamportClock{},

		// Lamport mutual exclusion
		lm:                NewLamportMutexQueue(),
		lockRequestLC:     -1,
		lockAckCounter:    0,
		lockRequestActive: false,

		// Flooding
		known: map[string]struct{}{},

		// Transaction balance
		balance: rand.Intn(100000), // random value between 0 and 100k
		randP:   0,                 // updated on every request
	}, "BANKING"
}

func (b *banking) Preflight(ctx context.Context, h *handler) error {
	go b.leaderLoop(ctx, h)
	go b.transactionLoop(ctx, h)
	return nil
}

// floodWithLamport is a simple network flooding, increasing the lamport clock for every transmitted message
func (b *banking) floodWithLamportClock(h *handler, msg *com.Message) int {
	counter := 0

	rUID, err := nthString(*msg.Payload, 2)
	if err != nil {
		log.Err(err).Msgf("failed to flood, no message uid; %s", *msg.Payload)
		return counter
	}

	if _, ok := b.known[rUID]; ok {
		log.Debug().Msgf("Already known, %s", *msg.Payload)
		return counter
	} else {
		b.known[rUID] = struct{}{}
	}

	for nuid, connect := range h.neighs.Nodes {
		if nuid == *msg.SourceUID {
			continue
		}
		ss := strings.Split(*msg.Payload, ";")
		if len(ss) < 2 {
			log.Error().Msgf("payload incompatible with lamport propagate %s", *msg.Payload)
			continue
		}
		ss[1] = fmt.Sprint(b.lc.Tick())
		msg.Payload = com.StrPointer(strings.Join(ss, ";"))

		// Send message
		if err := com.Send(connect, com.MsgPropagate(h.uid, msg)); err != nil {
			log.Err(err).Msg("Failed to propagate")
		} else {
			counter = counter + 1
		}
	}

	return counter
}

func (b *banking) Handle(h *handler, msg *com.Message) error {
	// Try to handle leader elect message, those do not have timestamps attached to them
	if ok, err := b.leader.TryHandleLeaderMessage(h, msg); ok {
		return err
	}

	// Update Lamport Clock
	if msgLC, err := nthInt(*msg.Payload, 1); err != nil {
		return err
	} else {
		b.lc.ReceiveEventTS(msgLC)
	}

	// Handle requests
	switch payload := *msg.Payload; {
	// distributed mutex
	case strings.HasPrefix(payload, "lockRequest"): // Check if payload is allowed
		return b.handle_lockRequest(h, msg)
	case strings.HasPrefix(payload, "lockAck"): // Check if payload is allowed
		return b.handle_lockAck(h, msg)
	case strings.HasPrefix(payload, "lockRelease"): // Check if payload is allowed
		return b.handle_lockRelease(h, msg)
	// transactions
	case strings.HasPrefix(payload, "transactStart"): // Check if payload is allowed
		return b.handle_transactStart(h, msg)
	case strings.HasPrefix(payload, "transactBalance"): // Check if payload is allowed
		return b.handle_transactBalance(h, msg)
	case strings.HasPrefix(payload, "transactGetBalance"): // Check if payload is allowed
		return b.handle_transactGetBalance(h, msg)
	case strings.HasPrefix(payload, "transactAck"): // Check if payload is allowed
		return b.handle_transactAck(h, msg)
	}

	log.Warn().Msgf("Payload `%s` not supported", *msg.Payload)

	return nil
}

// Perform regular distributed transactions
func (b *banking) transactionLoop(ctx context.Context, h *handler) error {

	// Block until leader collection is OK
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping transaction loop (banking)")
			return nil
		case <-time.After(50 * time.Millisecond):
		}
		if b.leader.ElectionComplete() {
			break
		}
	}

	log.Warn().Msg("starting transaction loop (banking)")

	for {
		// Sleep between 0 and 3 seconds
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping transaction loop (banking)")
			return nil
		case <-time.After(time.Duration(rand.Intn(3000)) * time.Millisecond):
		}

		// Aquire mutex lock
		reqLC := b.lc.Tick()
		b.lockAckCounter = 0
		b.lm.Add(reqLC, int(h.uid))
		b.distributeWithLamportClock(h, com.Msg(h.uid, "BANKING", fmt.Sprintf("lockRequest;<placeholder>;%d;%d", h.uid, reqLC)))

		// Block until lock acquired
		for {
			// Context aware sleep
			select {
			case <-ctx.Done():
				log.Info().Msg("Stopping transaction loop (banking)")
				return nil
			case <-time.After(50 * time.Millisecond):
			}
			if b.lockRequestActive {
				break
			}
		}
		log.Warn().Msg("ENTERING CRITICAL SECTION")

		// Initiate the transaction
		b.transactAckReceived = false
		b.transactBalanceReceived = false
		b.randP = rand.Intn(100)

		// Random neighbour
		randN := rand.Intn(len(h.neighs.AllNodes)) + 1
		for {
			if randN != int(h.uid) {
				break
			}
			randN = rand.Intn(len(h.neighs.AllNodes)) + 1
		}

		// Send start message
		reqStart := com.Msg(h.uid, "BANKING", fmt.Sprintf("transactStart;<placeholder>;%s;%d;%d;%d", uuid.NewString()[:8], randN, b.balance, b.randP))
		reqBalance := com.Msg(h.uid, "BANKING", fmt.Sprintf("transactGetBalance;<placeholder>;%s;%d", uuid.NewString()[:8], randN))

		log.Info().Msgf("Starting transaction with node %d; own balance: %d; random p: %d", randN, b.balance, b.randP)
		// Send messages (!! order matters here, turning it around would make a bit more sense)
		b.floodWithLamportClock(h, reqStart)
		b.floodWithLamportClock(h, reqBalance)

		// Wait for the conditions to be OK
		for {
			// Context aware sleep
			select {
			case <-ctx.Done():
				log.Info().Msg("Stopping transaction loop (banking)")
				return nil
			case <-time.After(1 * time.Second):
			}
			// We need to both perform the balance update on our and as well as want the other node to update its balance
			if b.transactAckReceived && b.transactBalanceReceived {
				break
			}
		}

		log.Warn().Msg("EXIT CRITICAL SECTION")
		// Release mutex lock
		b.lm.Pop()
		b.lockRequestActive = false
		b.distributeWithLamportClock(h, com.Msg(h.uid, "BANKING", fmt.Sprintf("lockRelease;<placeholder>;%d;%d", h.uid, reqLC)))
		// Check if there's another node requesting a lock
		if lockLC, lockNUID, ok := b.lm.Next(); ok {
			// Send ACK to the next node
			b.distributeWithLamportClock(h, com.Msg(h.uid, "BANKING", fmt.Sprintf("lockAck;<placeholder>;%d;%d", lockNUID, lockLC)))
		}

	}
}

// Leader election
func (b *banking) leaderLoop(ctx context.Context, h *handler) error {

	// Block until this node is leader
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping leader (banking)")
			return nil
		case <-time.After(50 * time.Millisecond):
		}
		if b.leader.IsLeader() {
			break
		} else if b.leader.ElectionComplete() {
			log.Warn().Msg("This node lost the election (banking)")
			return nil
		}
	}

	log.Warn().Msg("starting observer (banking)")
	return nil
}

// DistributeSpanningTree propagates messages along the spanning tree, more efficient compared to simple flooding
func (b *banking) distributeWithLamportClock(h *handler, msg *com.Message) int {
	if !b.leader.ElectionComplete() {
		log.Error().Msg("distribute requires a spanning tree; leader election not complete")
		return 0
	}

	spanningTreeNeighs := append(b.leader.childUIDs, b.leader.srcUID)
	total := 0
	for _, nuid := range spanningTreeNeighs {
		if nuid == *msg.SourceUID || nuid == h.uid {
			continue // skip sending to receiver
		}
		connect, ok := h.neighs.Nodes[nuid]
		if !ok {
			log.Error().Msgf("failed to find connect string for node with UID %d", nuid)
			continue
		}

		// Update lamport clock (also in payload)
		ss := strings.Split(*msg.Payload, ";")
		if len(ss) < 2 {
			log.Error().Msgf("payload incompatible with lamport propagate %s", *msg.Payload)
			continue
		}
		ss[1] = fmt.Sprint(b.lc.Tick())
		msg.Payload = com.StrPointer(strings.Join(ss, ";"))

		// Send message
		err := com.Send(connect, com.MsgPropagate(h.uid, msg))
		if err != nil {
			log.Err(err).Msg("Failed to proagate")
		}
		total += 1
	}

	return total
}

// ==== Lamport Mutual Exclusion

// handle_lockRequest handles the lock requests
func (b *banking) handle_lockRequest(h *handler, msg *com.Message) error {

	lockNUID, err := nthInt(*msg.Payload, 2)
	if err != nil {
		return err
	}
	lockLC, err := nthInt(*msg.Payload, 3)
	if err != nil {
		return err
	}

	if ok, err := b.lm.Add(lockLC, lockNUID); ok {
		// directly distribute ACK
		b.distributeWithLamportClock(h, com.Msg(h.uid, "BANKING", fmt.Sprintf("lockAck;<placeholder>;%d;%d", lockNUID, lockLC)))
	} else if err != nil {
		return err
	}

	// Distribute the message across the spanning tree
	b.distributeWithLamportClock(h, msg)
	return nil
}

// handle_lockRequest handles the lock requests
func (b *banking) handle_lockRelease(h *handler, msg *com.Message) error {

	lockNUID, err := nthInt(*msg.Payload, 2)
	if err != nil {
		return err
	}
	lockLC, err := nthInt(*msg.Payload, 3)
	if err != nil {
		return err
	}

	qLC, qNUID, ok := b.lm.Pop()
	if !ok || (qLC != lockLC && qNUID != lockNUID) {
		log.Error().Msgf("%t %d %d !== %d %d", ok, qLC, qNUID, lockLC, lockNUID)
		return errors.New("lockRelease invalid state")
	}

	// Check if we should send the next ACK for the next waiting lock entry
	if lockLC, lockNUID, ok := b.lm.Next(); ok {
		// directly distribute ACK
		b.distributeWithLamportClock(h, com.Msg(h.uid, "BANKING", fmt.Sprintf("lockAck;<placeholder>;%d;%d", lockNUID, lockLC)))
	}

	// Distribute the message across the spanning tree
	b.distributeWithLamportClock(h, msg)
	return nil
}

func (b *banking) handle_lockAck(h *handler, msg *com.Message) error {

	reqLC, err := nthInt(*msg.Payload, 1)
	if err != nil {
		return err
	}
	lockNUID, err := nthInt(*msg.Payload, 2)
	if err != nil {
		return err
	}
	lockLC, err := nthInt(*msg.Payload, 3)
	if err != nil {
		return err
	}

	// Only affects if this node requested the lock
	if lockNUID == int(h.uid) {
		// Update local state -> we have the lock
		if reqLC > lockLC {
			b.lockAckCounter = b.lockAckCounter + 1
		}
		if n := len(h.neighs.AllNodes); b.lockAckCounter == n {
			b.lockRequestActive = true
			log.Info().Msg("Lamport Mutex lock active on this node")
		} else {
			log.Info().Msgf("Received ack from %d/%d nodes", b.lockAckCounter, n)
		}
	} else {
		// Distribute the message across the spanning tree
		b.distributeWithLamportClock(h, msg)
	}

	return nil
}

func (b *banking) handle_transactStart(h *handler, msg *com.Message) error {
	rUID, err := nthString(*msg.Payload, 2)
	if err != nil {
		return err
	}
	targetID, err := nthInt(*msg.Payload, 3)
	if err != nil {
		return err
	}
	balance, err := nthInt(*msg.Payload, 4)
	if err != nil {
		return err
	}
	p, err := nthInt(*msg.Payload, 5)
	if err != nil {
		return err
	}

	// Make sure this is only handled once
	if _, ok := b.known[rUID]; ok {
		log.Debug().Msg("Deduplicating transactStart request")
		return nil
	}

	// Check if this node was asked; if so, return
	if targetID == int(h.uid) {
		oldBalance := b.balance
		// Update own balance according to the rules
		if balance >= b.balance {
			b.balance = b.balance + (balance/100)*p
		} else {

			b.balance = b.balance - (balance/100)*p
		}
		log.Info().Msgf("Updated balance from %d to %d", oldBalance, b.balance)

		resp := com.Msg(h.uid, "BANKING", fmt.Sprintf("transactAck;<placeholder>;%s", uuid.NewString()[:8]))
		b.known[rUID] = struct{}{}
		b.floodWithLamportClock(h, resp)
	} else {
		b.floodWithLamportClock(h, msg)
	}

	return nil
}

func (b *banking) handle_transactAck(h *handler, msg *com.Message) error {
	rUID, err := nthString(*msg.Payload, 2)
	if err != nil {
		return err
	}

	// Make sure this is only handled once
	if _, ok := b.known[rUID]; ok {
		log.Debug().Msg("Deduplicating transact ack response")
		return nil
	}

	if b.lockRequestActive {
		b.known[rUID] = struct{}{}
		b.transactAckReceived = true
	} else {
		b.floodWithLamportClock(h, msg)
	}

	return nil
}

func (b *banking) handle_transactGetBalance(h *handler, msg *com.Message) error {
	rUID, err := nthString(*msg.Payload, 2)
	if err != nil {
		return err
	}
	targetID, err := nthInt(*msg.Payload, 3)
	if err != nil {
		return err
	}

	// Make sure this is only handled once
	if _, ok := b.known[rUID]; ok {
		log.Debug().Msg("Deduplicating balance response")
		return nil
	}

	// Check if this node was asked; if so, return
	if targetID == int(h.uid) {
		resp := com.Msg(h.uid, "BANKING", fmt.Sprintf("transactBalance;<placeholder>;%s;%d", uuid.NewString()[:8], b.balance))
		b.known[rUID] = struct{}{}
		b.floodWithLamportClock(h, resp)
	} else {
		b.floodWithLamportClock(h, msg)
	}

	return nil
}

func (b *banking) handle_transactBalance(h *handler, msg *com.Message) error {
	// Mutex; no need to check neigh IDs
	rUID, err := nthString(*msg.Payload, 2)
	if err != nil {
		return err
	}
	balance, err := nthInt(*msg.Payload, 3)
	if err != nil {
		return err
	}

	// Make sure this is only handled once
	if _, ok := b.known[rUID]; ok {
		log.Debug().Msg("Deduplicating balance response")
		return nil
	}

	// this node is supposed to update our balance!
	oldBalance := b.balance
	if b.lockRequestActive {
		if balance >= b.balance {
			b.balance = b.balance + (balance/100)*b.randP
		} else {

			b.balance = b.balance - (balance/100)*b.randP
		}
		log.Info().Msgf("Updated balance from %d to %d", oldBalance, b.balance)
		b.transactBalanceReceived = true // Update so the transactLoop can continue

		// Make sure we ignore future messages here
		b.known[rUID] = struct{}{}
	} else {
		// Flood until we reach the destination
		b.floodWithLamportClock(h, msg)
	}

	return nil
}
