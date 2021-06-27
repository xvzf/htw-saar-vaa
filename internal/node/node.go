package node

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
	"github.com/xvzf/vaa/pkg/neigh"
)

type Handler interface {
	Run(context.Context, chan *com.Message) error
	Register(Extension, string)
}

// handler holds internal information & datastructures for a node
type handler struct {
	uid    uint
	neighs *neigh.Neighs
	exit   context.CancelFunc
	wg     sync.WaitGroup
	ext    map[string]Extension
}

func New(uid uint, exitFunc context.CancelFunc, neighs *neigh.Neighs) Handler {
	// Init datastructures of the node
	return &handler{
		uid:    uid,
		exit:   exitFunc,
		wg:     sync.WaitGroup{},
		neighs: neighs,
		ext:    make(map[string]Extension),
	}
}

func (h *handler) Register(e Extension, t string) {
	log.Info().Uint("uid", h.uid).Msgf("Registered handler for type %s", t)
	h.ext[t] = e
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
		Time("timestamp", *msg.Timestamp).
		Uint("src_uid", *msg.SourceUID).
		Str("type", *msg.Type).
		Str("payload", *msg.Payload).
		Msg("<<<")

	// Mark processing start/end; this allows us to gracefully shutdown on context cancelation
	h.wg.Add(1)
	defer h.wg.Done()

	log.Debug().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msg("Routing to correct handler")

	// Pass to extension
	if e, ok := h.ext[*msg.Type]; ok {
		return e.Handle(h, msg)
	}

	log.Warn().Uint("uid", h.uid).Uint("src_uid", *msg.SourceUID).Str("req_id", *msg.UUID).Msgf("Message type `%s` not supported", *msg.Type)
	return nil
}
