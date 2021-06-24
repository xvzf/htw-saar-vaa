package com

import (
	"context"
	"encoding/json"
	"net"
	"strings"

	"github.com/rs/zerolog/log"
)

// Dispatcher
type Dispatcher interface {
	Run(context.Context) error
}

type listenConfig struct {
	listen     string
	handleChan chan *Message
}

// NewDispatcher create a new Server dispatching messages to a go channel
func NewDispatcher(listen string, handleChan chan *Message) Dispatcher {
	return &listenConfig{
		listen:     listen,
		handleChan: handleChan,
	}
}

// handleConn handles incoming connections, decodes the message and sends it to a channel
func (c *listenConfig) handleConn(ctx context.Context, conn net.Conn) {
	// Generate unique identifier for the incoming request
	log.Debug().
		Msgf("Handling incomming connection from %s", conn.RemoteAddr().String())
	defer conn.Close()

	// Decode the incoming payload
	msg := &Message{}
	d := json.NewDecoder(conn)
	err := d.Decode(msg)
	if err != nil {
		log.Err(err).Msg("failed to decode incoming message")
		return
	}

	// Verify the message is valid
	if err := msg.isValid(); err != nil {
		log.Err(err).Msg("received invalid message")
		return
	}

	// All OK, log full message content
	log.Info().
		Str("msg_direction", "incoming").
		Str("req_id", *msg.UUID).
		Uint("ttl", *msg.TTL).
		Time("timestamp", *msg.Timestamp).
		Uint("src_uid", *msg.SourceUID).
		Str("type", *msg.Type).
		Msg(*msg.Payload)

	// Propagate the message to channel in case our context is not closed yet
	select {
	case <-ctx.Done():
		// When the context is closed we don't want to propagate the message
		return
	default:
		// We're still live, propagatae message
		log.Debug().Msg("Sending message to channel")
		c.handleChan <- msg
	}
}

// Run starts a TCP server on a configured port and dispatches messages to a specified go channel
func (c *listenConfig) Run(ctx context.Context) error {
	log.Info().Msgf("Start listening on %s", c.listen)

	l, err := net.Listen("tcp", c.listen)
	if err != nil {
		log.Err(err).Msg("failed to construct listener")
		return err
	}

	// Handle Context cancel/timeout
	go func() {
		defer l.Close()
		<-ctx.Done()
		log.Info().Msgf("Stop listening on %s", c.listen)
	}()

	// Accept incoming connection
	for {
		conn, err := l.Accept()
		if err != nil {
			// Check if connection has been shutdown
			if strings.Contains(err.Error(), "use of closed network connection") {
				log.Info().Msg("Stopped listen loop")
				return nil
			}
			log.Err(err).Msg("failed to accept connection")
			return err
		}

		// Dispatch connection to connection handler
		go c.handleConn(ctx, conn)
	}
}
