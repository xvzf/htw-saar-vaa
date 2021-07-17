package node

import (
	"fmt"
	"sort"
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
		log.Debug().Msgf("Lamport Clock updated (receive event); LC=%d", c.LC)
	}
}

// Tick bumps the lamport clock and returns the value
func (c *lamportClock) Tick() int {
	c.Lock()
	defer c.Unlock()
	c.LC = c.LC + 1
	log.Debug().Msgf("Lamport Clock updated (tick); LC=%d", c.LC)
	return c.LC
}

// lamportMutex provides a simple datastructure for keepint track of lock requests
type lamportMutexQueue struct {
	sync.Mutex
	tsNodeMap map[int]int // map timestamp -> node_uid
	queue     []int       // keep track of lock requests
}

func NewLamportMutexQueue() *lamportMutexQueue {
	return &lamportMutexQueue{
		tsNodeMap: make(map[int]int),
		queue:     []int{},
	}
}

func (lm *lamportMutexQueue) Add(ts, nuid int) (bool, error) {
	// Check if the timestamps is already in the queue
	for _, e := range lm.queue {
		if e == ts {
			return false, fmt.Errorf("timestamp %d already in queue, duplicate?", ts)
		}
	}
	// Update queue
	lm.queue = append(lm.queue, ts)
	lm.tsNodeMap[ts] = nuid

	// Sort priority queue
	sort.Ints(lm.queue)

	log.Debug().Msgf("LamportLock Queue" + fmt.Sprint(lm.queue))
	log.Debug().Msgf("LamportLock Map" + fmt.Sprint(lm.tsNodeMap))

	log.Info().Msgf("LamportLock added node=%d ts=%d", nuid, ts)

	// Check if the req
	return (lm.queue[0] == ts), nil
}

func (lm *lamportMutexQueue) Pop() (int, int, bool) {

	// Check if there's anything in the queue
	if len(lm.queue) < 1 {
		return 0, 0, false
	}

	// Get ts <-> node pair
	ts := lm.queue[0]
	lm.queue = lm.queue[1:]
	nuid := lm.tsNodeMap[ts]
	delete(lm.tsNodeMap, ts)

	log.Info().Msgf("LamportLock released node=%d ts=%d", nuid, ts)

	return ts, nuid, true
}

func (lm *lamportMutexQueue) Next() (int, int, bool) {

	// Check if there's anything in the queue
	if len(lm.queue) < 1 {
		return 0, 0, false
	}

	// Get ts <-> node pair
	ts := lm.queue[0]
	nuid := lm.tsNodeMap[ts]

	return ts, nuid, true
}
