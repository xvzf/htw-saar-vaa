package node

import (
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
)

// rumor holds the datastructure used to work on a rumor
type rumor struct {
	sync.Mutex
	counter       map[string]int
	trustedRumors map[string]bool
}

// NewRumorExtension returns the rumor handler + the message type
func NewRumorExtension() (Extension, string) {
	return &rumor{
		counter:       make(map[string]int),
		trustedRumors: make(map[string]bool),
	}, "RUMOR"
}

// Add adds a new or existing rumor to the datastructure. It returns the number of registered rumors
func (r *rumor) add(rumor string) int {
	r.Lock()
	defer r.Unlock()

	v, ok := r.counter[rumor]
	if ok {
		r.counter[rumor] = v + 1
	} else {
		r.counter[rumor] = 1
	}

	return r.counter[rumor]
}

func (r *rumor) trusted(rumor string) {
	r.Lock()
	defer r.Unlock()
	r.trustedRumors[rumor] = true
}

// handleRumor handles incoming rumor events
func (r *rumor) Handle(h *handler, msg *com.Message) error {
	log.Debug().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Handling rumor message")
	ps := strings.Split(*msg.Payload, ";")
	if len(ps) != 2 {
		log.Error().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Invalid message payload")
		return errors.New("invalid message payload")
	}

	// Extract payload
	rm := ps[1]
	c, err := strconv.Atoi(ps[0])
	if err != nil {
		log.Err(err).Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("C invalid")
		return err
	}

	// Increase rumor counter
	s := r.add(rm)

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

	log.Info().Uint("uid", h.uid).Str("rumor", rm).Int("seen", s).
		Msgf("Counter increased")

	if s == c { // Initially trusted
		r.trusted(rm)
		log.Info().Uint("uid", h.uid).Str("rumor", rm).Int("seen", s).
			Msgf("Now trusted")
	} else if s > c { // Already trusted
		log.Debug().Uint("uid", h.uid).Str("rumor", rm).Int("seen", s).
			Msgf("Trusted since %d shares", s-c)
	}
	return nil
}
