package com

import (
	"encoding/json"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

func Send(target string, msg *Message) error {
	// Assign UUID to outgoing request for easier tracing in other nodes
	u := uuid.New()
	ustr := u.String()
	msg.UUID = &ustr

	log.Debug().
		Str("req_id", u.String()).
		Msgf("Sending request to %s", target)

	// Time limit so we don't go stale
	conn, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Transmit json payload
	e := json.NewEncoder(conn)
	err = e.Encode(msg)
	if err != nil {
		return err
	}

	log.Info().
		Str("msg_direction", "outgoing").
		Str("req_id", *msg.UUID).
		Uint("ttl", *msg.TTL).
		Time("timestamp", *msg.Timestamp).
		Uint("src_uid", *msg.SourceUID).
		Str("type", *msg.Type).
		Str("payload", *msg.Payload).
		Msg("sent")

	return nil
}
