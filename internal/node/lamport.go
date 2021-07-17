package node

import (
	"sync"

	"github.com/rs/zerolog/log"
)

// lClock implements the lamport clock
type lamportClock struct {
	sync.Mutex
	LC int
}

// ReceiveEventTS updates the lamport clock based on a received message
func (c *lamportClock) ReceiveEventTS(ts int) {
	c.Lock()
	defer c.Unlock()
	if ts > c.LC {
		c.LC = ts + 1
		log.Info().Msgf("Lamport Clock updated (receive event); LC=%d", c.LC)
	}
}

// Tick bumps the lamport clock and returns the value
func (c *lamportClock) Tick() int {
	c.Lock()
	defer c.Unlock()
	c.LC = c.LC + 1
	log.Info().Msgf("Lamport Clock updated (tick); LC=%d", c.LC)
	return c.LC
}
