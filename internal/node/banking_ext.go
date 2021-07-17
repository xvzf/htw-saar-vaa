package node

import (
	"context"
	"math/rand"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
)

// Distributed Banking
type banking struct {
	// Vars used for leader election
	leader *Leader

	// Lamport clock
	lc *lamportClock
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
	}, "BANKING"
}

func (b *banking) Preflight(ctx context.Context, h *handler) error {
	go b.leaderLoop(ctx, h)
	return nil
}

func (b *banking) Handle(h *handler, msg *com.Message) error {
	// Try to handle leader elect message
	if ok, err := b.leader.TryHandleLeaderMessage(h, msg); ok {
		return err
	}

	return nil
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

	log.Warn().Msg("this node is now leader (banking)")

	return nil
}
