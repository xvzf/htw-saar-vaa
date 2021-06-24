package com

import (
	"errors"
	"time"
)

type Message struct {
	UUID      *string    `json:"uuid"`
	TTL       *uint      `json:"uint"`
	Timestamp *time.Time `json:"timestamp"`
	SourceUID *uint      `json:"src_uid"`
	Type      *string    `json:"type"`
	Payload   *string    `json:"payload"`
}

// Checks if all fields have been set
func (m *Message) isValid() error {
	// Input checking
	if m.UUID == nil {
		return errors.New("UUID not set")
	}
	if m.TTL == nil {
		return errors.New("TTL not set")
	}
	if m.Timestamp == nil {
		return errors.New("timestamp not set")
	}
	if m.SourceUID == nil {
		return errors.New("SourceUID not set")
	}
	if m.Type == nil {
		return errors.New("type not set")
	}
	if m.Payload == nil {
		return errors.New("payload not set")
	}
	return nil
}

func strPointer(s string) *string {
	return &s
}

func uintPointer(i uint) *uint {
	return &i
}

func timePointer(t time.Time) *time.Time {
	return &t
}

// Msg is a handy wrapper constructing a message absed on originating uid, type and payload
func Msg(uid uint, msgType, msgPayload string) *Message {
	return &Message{
		TTL:       uintPointer(1),
		Timestamp: timePointer(time.Now().UTC()),
		SourceUID: uintPointer(uid),
		Type:      strPointer(msgType),
		Payload:   strPointer(msgPayload),
	}
}
