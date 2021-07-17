package node

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
)

type consensusState struct {
	sync.Mutex
	active        bool
	msgInCounter  int
	msgOutCounter int
}

func (s *consensusState) Received() {
	s.Lock()
	defer s.Unlock()
	s.msgInCounter = s.msgInCounter + 1
}

func (s *consensusState) Sent() {
	s.Lock()
	defer s.Unlock()
	s.msgOutCounter = s.msgOutCounter + 1
}

func (s *consensusState) Add(s2 *consensusState) {
	s.Lock()
	defer s.Unlock()
	s2.Lock()
	defer s2.Unlock()
	// Goroutine safe addition
	s.active = s.active || s2.active
	s.msgInCounter = s.msgInCounter + s2.msgInCounter
	s.msgOutCounter = s.msgOutCounter + s2.msgOutCounter
}

type resultState struct {
	sync.Mutex
	agreement bool
	timestamp int
}

func (r *resultState) Add(agreement bool, timestamp int) {
	r.Lock()
	defer r.Unlock()

	r.agreement = agreement && r.agreement

	if r.timestamp != -1 && timestamp != -1 && timestamp != r.timestamp {
		r.agreement = false
	} else if r.timestamp == -1 {
		r.timestamp = timestamp
	}

	if !r.agreement {
		r.timestamp = -1
	}

}

type consensus struct {
	leader *Leader

	// Echo communication for state/collect requests.
	echo map[string]int // map[<req_uid>]<counter_recv_messages>

	// Alignment on a common discrete time
	sVote    int // number of nodes initiating the voting process
	pNeighs  int // number of random neighs to agree on a time
	aMax     int // voting rounds accepted
	aCurrent int // voting rounds accepted
	tK       int // discrete time of this node

	// State
	state        *consensusState            // This node state
	accState     map[string]*consensusState // Accumulated state for state requests
	accStateDone map[string]bool            // Accumulated state for state requests

	// Result return
	accResult     map[string]*resultState
	accResultDone map[string]bool
}

func NewConsensusExtension(s, m, p, aMax int) (Extension, string) {
	rand.Seed(time.Now().UnixNano())
	wantLeader := rand.Intn(2) == 1 // 50% chance of being true
	log.Info().Msgf("Wants to be leader: %t", wantLeader)
	return &consensus{
		leader: NewLeader("CONSENSUS", wantLeader),

		// Echo communication
		echo: map[string]int{},

		// Discrete timestamp
		sVote:    s,
		tK:       rand.Intn(m) + 1,
		aMax:     aMax,
		aCurrent: 0,
		pNeighs:  p,

		// State
		state: &consensusState{
			active:        false,
			msgInCounter:  0,
			msgOutCounter: 0,
		},

		accState:      make(map[string]*consensusState),
		accStateDone:  make(map[string]bool),
		accResult:     make(map[string]*resultState),
		accResultDone: make(map[string]bool),
	}, "CONSENSUS"
}

func (c *consensus) Preflight(ctx context.Context, h *handler) error {
	go c.leaderLoop(ctx, h)
	return nil
}

func (c *consensus) Handle(h *handler, msg *com.Message) error {
	// Try to handle leader elect message
	if ok, err := c.leader.TryHandleLeaderMessage(h, msg); ok {
		return err
	}
	// If not handled, continue with the consensus messages
	switch payload := *msg.Payload; {
	// State Request needs to be checked before the response
	case strings.HasPrefix(payload, "stateRequest"): // Check if payload is allowed
		return c.handle_stateRequest(h, msg)
	case strings.HasPrefix(payload, "stateResponse"): // Check if payload is allowed
		return c.handle_stateResponse(h, msg)
	// Vote requests
	case strings.HasPrefix(payload, "voteBegin"): // Check if payload is allowed
		return c.handle_voteBegin(h, msg)
	case strings.HasPrefix(payload, "proposalResponse"): // Check if payload is allowed
		return c.handle_proposalResponse(h, msg)
	case strings.HasPrefix(payload, "proposal"): // Check if payload is allowed
		return c.handle_proposal(h, msg)
	// collect requests
	case strings.HasPrefix(payload, "collectRequest"): // Check if payload is allowed
		return c.handle_collectRequest(h, msg)
	case strings.HasPrefix(payload, "collect"): // Check if payload is allowed
		return c.handle_collect(h, msg)
	}

	return fmt.Errorf("payload `%s` not supported", *msg.Payload)
}

func randNeighsUnique(in map[uint]string, p int) []string {
	n := []string{}
	r := []string{}
	randNodes := map[string]struct{}{}

	for _, v := range in {
		n = append(n, v)
	}

	// safety check (doesn't cover all cases)
	if p > len(n) {
		log.Error().Msgf("p (%d) > number of available nodes (%d)", p, len(n))
		return r
	}

	// Unique random neighs
	for len(randNodes) < p {
		randNodes[n[rand.Intn(len(n))]] = struct{}{}
	}

	// Convert to array again
	for v := range randNodes {
		r = append(n, v)
	}

	return r
}

// leader is the control method
func (c *consensus) leaderLoop(ctx context.Context, h *handler) error {
	// Create IDs
	var prevStateID string = ""
	var currStateID string = ""

	// Block until this node is leader
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping leader (consensus)")
			return nil
		case <-time.After(50 * time.Millisecond):
		}
		if c.leader.IsLeader() {
			break
		} else if c.leader.ElectionComplete() {
			log.Warn().Msg("This node lost the election (consensus)")
			return nil
		}
	}

	log.Warn().Msg("this node is now leader (consensus)")

	// Select up to S random philosophs (max number of neigh) to initiate the voting process
	if c.sVote > len(h.neighs.Nodes) {
		c.sVote = len(h.neighs.Nodes)
	}

	m := com.Msg(h.uid, "CONSENSUS", "voteBegin")
	for _, connect := range randNeighsUnique(h.neighs.Nodes, c.sVote) {
		log.Info().Msgf("Send voteBegin to %s", connect)

		if err := com.Send(connect, m); err != nil {
			log.Err(err).Msg("Failed to send voteBegin message")
		} else {
			c.state.Sent()
		}
	}

	// Perform Double Counting until the two reported, consecutive states match
	for {

		// Some sleeps between the interval
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping leader")
			return nil
		case <-time.After(1 * time.Second):
		}

		// Check state; updated by receiving node process
		_, okPrev := c.accStateDone[prevStateID]
		_, okCurr := c.accStateDone[currStateID]
		statePrev := c.accState[prevStateID]
		stateCurr := c.accState[currStateID]

		if currStateID != "" && !okCurr {
			// Wait for state to be reported
			log.Info().Msgf("Waiting for state to come in; id %s", currStateID)
			continue
		} else {
			// First iteration or the state received

			// Compare current and last state in case they both exist
			if okPrev && okCurr && statePrev.msgInCounter == statePrev.msgOutCounter && stateCurr.msgInCounter == stateCurr.msgOutCounter {
				log.Info().Msg("State Converged")
				break
			} else {
				// rotate
				prevStateID = currStateID
				currStateID = uuid.NewString()[0:8]
				log.Info().Msgf("double counting mismatch; starting state collection with id %s", currStateID)

				// c.echo[currStateID] = 0 FIXME
				c.echo[currStateID] = -1
				c.accState[currStateID] = &consensusState{active: false, msgInCounter: 0, msgOutCounter: 0}
				m := com.Msg(h.uid, "CONSENSUS", "stateRequest;"+currStateID)
				_ = c.leader.propagateChilds(h, m)
			}
		}
	}

	// Collect results
	collectID := uuid.NewString()[0:8]
	mCollect := com.Msg(h.uid, "CONSENSUS", "collectRequest;"+collectID)
	c.echo[collectID] = 0
	c.accResult[collectID] = &resultState{agreement: true, timestamp: -1}
	_ = c.leader.propagateChilds(h, mCollect)
	for {

		// Some sleeps between the interval
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping leader")
			return nil
		case <-time.After(1 * time.Second):
		}

		_, ok := c.accResultDone[collectID]
		if ok {
			if res, ok := c.accResult[collectID]; ok {
				log.Info().Msgf("Agreement: %t, (timestamp: %d)", res.agreement, res.timestamp)
			} else {
				err := fmt.Errorf("result not available, internal error %s", collectID)
				log.Err(err).Msg("invalid state")
				return err
			}
			// We're done here, get out
			break
		}
		log.Info().Msg("Consensus leader waiting for collect result")
	}
	log.Warn().Msg("Consensus Leader exited")
	return nil
}

func (c *consensus) sendProposals(h *handler) {

	if c.pNeighs > len(h.neighs.Nodes) {
		log.Info().Msgf("Correcting pNeighs (%d) to %d due to neighbour limitations", c.pNeighs, len(h.neighs.Nodes))
		c.pNeighs = len(h.neighs.Nodes)
	}

	m := com.Msg(h.uid, "CONSENSUS", fmt.Sprintf("proposal;%d", c.tK))

	// Send requests
	for _, connect := range randNeighsUnique(h.neighs.Nodes, c.pNeighs) {
		// Sleep random time to avoid connection timeouts
		// time.Sleep(time.Duration(rand.Intn(200)) * time.Millisecond)
		if err := com.Send(connect, m); err != nil {
			log.Err(err).Msgf("Sent proposal to %s", connect)
		} else {
			c.state.Sent()
		}
	}
}

func (c *consensus) handle_voteBegin(h *handler, msg *com.Message) error {
	log.Info().Msg("Start voting")
	c.state.Received()

	// Send requests to random neighs
	c.sendProposals(h)

	return nil
}

func (c *consensus) handle_proposal(h *handler, msg *com.Message) error {
	c.state.Received()

	if c.aCurrent >= c.aMax {
		log.Info().Msg("this node is not accepting further proposals")
		return nil
	}
	c.aCurrent = c.aCurrent + 1

	proposedTime, err := nthInt(*msg.Payload, 1)
	if err != nil {
		return err
	}

	// Proposal incoming; calculate mid time and send response
	newT := int(math.Ceil((float64(proposedTime) + float64(c.tK)) / 2))
	log.Info().Msgf("New t_k = %d; (old = %d)", newT, c.tK)
	c.tK = newT

	// Send response
	log.Info().Msgf("Sending proposalResponse to uid %d", *msg.SourceUID)
	m := com.Msg(h.uid, "CONSENSUS", fmt.Sprintf("proposalResponse;%d", c.tK))
	if err := com.Send(h.neighs.Nodes[*msg.SourceUID], m); err != nil {
		log.Err(err).Msg("Failed to send proposalResponse message")
	} else {
		c.state.Sent()
	}

	// Start voting with random neighbours
	c.sendProposals(h)

	return nil
}

// handle_proposalResponse stores the agreed value
func (c *consensus) handle_proposalResponse(h *handler, msg *com.Message) error {
	c.state.Received()

	agreedTime, err := nthInt(*msg.Payload, 1)
	if err != nil {
		return err
	}

	log.Info().Msgf("Accepted agreed t_k = %d; (old = %d)", agreedTime, c.tK)
	c.tK = agreedTime

	return nil
}

func (c *consensus) resultReturn(h *handler, rUID string) {
	// Retrieve current state
	resultReceivedCount, ok := c.echo[rUID]
	if !ok {
		log.Error().Msgf("resultRequest %s does not exist", rUID)
	}
	resultState, ok := c.accResult[rUID]
	if !ok || resultState == nil {
		log.Error().Msgf("resultRequest %s does not exist or accResult not initialized", rUID)
	}

	// Forward echo
	if resultReceivedCount == len(c.leader.childUIDs) {
		// Add this node state to accState

		c.accResult[rUID].Add(true, c.tK)

		// Construct message & send it
		if c.leader.IsLeader() {
			log.Info().Msgf("Received final result for %s; (agreement: %t, timestamp: %d)", rUID, resultState.agreement, resultState.timestamp)
			c.accResultDone[rUID] = true
		} else {
			sMsg := com.Msg(h.uid, "CONSENSUS", fmt.Sprintf("collect;%s;%t;%d", rUID, resultState.agreement, resultState.timestamp))
			log.Info().Msgf("Propagate (accumulated) result to %d", c.leader.srcUID)
			com.Send(h.neighs.Nodes[c.leader.srcUID], sMsg)
		}

		/*
			// Cleanup the maps (prevent leakage)
			delete(c.echo, sUID)
			delete(c.accState, sUID)
		*/
	}

}

// Handle_state is used for identifying via the double counting method, if all nodes proceeded
func (c *consensus) handle_collectRequest(h *handler, msg *com.Message) error {
	// This message requires a leader (-> initialised spanning tree) to be present
	if !c.leader.ElectionComplete() {
		return errors.New("no leader in network")
	}
	rUID, err := nthString(*msg.Payload, 1)
	if err != nil {
		return err
	}

	// Init response counter (if not existing yet); then propagate to childs
	if _, ok := c.echo[rUID]; ok {
		return fmt.Errorf("state request with uid %s already exists", rUID)
	}
	// Initialize echo-based state collection
	c.echo[rUID] = 0
	c.accResult[rUID] = &resultState{
		agreement: true,
		timestamp: c.tK,
	}

	_ = c.leader.propagateChilds(h, msg)

	c.resultReturn(h, rUID)
	return nil
}

// Handle_state is used for identifying via the double counting method, if all nodes proceeded
func (c *consensus) handle_collect(h *handler, msg *com.Message) error {
	// This message requires a leader (-> initialised spanning tree) to be present
	if !c.leader.ElectionComplete() {
		return errors.New("no leader in network")
	}
	rUID, err := nthString(*msg.Payload, 1)
	if err != nil {
		return err
	}
	isActive, err := nthBool(*msg.Payload, 2)
	if err != nil {
		return err
	}
	timestamp, err := nthInt(*msg.Payload, 3)
	if err != nil {
		return err
	}
	eCount, ok := c.echo[rUID]
	if !ok {
		return fmt.Errorf("collect request with uid %s does not exists", rUID)
	}
	accResult, ok := c.accResult[rUID]
	if !ok || accResult == nil {
		return fmt.Errorf("collect request with uid %s does not exists or accState not initialized", rUID)
	}

	c.echo[rUID] = eCount + 1
	accResult.Add(isActive, timestamp)

	c.resultReturn(h, rUID)

	return nil
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
	if stateReceivedCount == len(c.leader.childUIDs) {
		// Add this node state to accState
		accState.Add(c.state)

		// Construct message & send it
		if c.leader.IsLeader() {
			log.Info().Msgf("Final state; (%s, %t, %d, %d)", sUID, accState.active, accState.msgInCounter, accState.msgOutCounter)
			c.accStateDone[sUID] = true
		} else {
			sMsg := com.Msg(h.uid, "CONSENSUS", fmt.Sprintf("stateResponse;%s;%t;%d;%d", sUID, accState.active, accState.msgInCounter, accState.msgOutCounter))
			log.Info().Msgf("Propagate (accumulated) state to %d", c.leader.srcUID)
			com.Send(h.neighs.Nodes[c.leader.srcUID], sMsg)
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
	if !c.leader.ElectionComplete() {
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
	_ = c.leader.propagateChilds(h, msg)

	// Check if we should return state early (-> when leaf node)
	c.stateReturn(h, sUID)
	return nil
}

// Handle_state is used for identifying via the double counting method, if all nodes proceeded
func (c *consensus) handle_stateResponse(h *handler, msg *com.Message) error {
	// This message requires a leader (-> initialised spanning tree) to be present
	if !c.leader.ElectionComplete() {
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
