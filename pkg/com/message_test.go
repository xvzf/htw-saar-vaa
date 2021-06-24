package com

import (
	"testing"
	"time"
)

func TestMessage_isValid(t *testing.T) {
	tests := []struct {
		name    string
		m       *Message
		wantErr bool
	}{
		{
			"Message OK",
			&Message{
				UUID:      new(string),
				Timestamp: new(time.Time),
				SourceUID: new(uint),
				Type:      new(string),
				Payload:   new(string),
			},
			false,
		},
		{
			"UUID missing",
			&Message{
				UUID:      nil,
				Timestamp: new(time.Time),
				SourceUID: new(uint),
				Type:      new(string),
				Payload:   new(string),
			},
			true,
		},
		{
			"Missing Timestamp",
			&Message{
				UUID:      new(string),
				Timestamp: nil,
				SourceUID: new(uint),
				Type:      new(string),
				Payload:   new(string),
			},
			true,
		},
		{
			"Missing SourceUID",
			&Message{
				UUID:      new(string),
				Timestamp: new(time.Time),
				SourceUID: nil,
				Type:      new(string),
				Payload:   new(string),
			},
			true,
		},
		{
			"Missign Type",
			&Message{
				UUID:      new(string),
				Timestamp: new(time.Time),
				SourceUID: new(uint),
				Type:      nil,
				Payload:   new(string),
			},
			true,
		},
		{
			"Missing Payload",
			&Message{
				UUID:      new(string),
				Timestamp: new(time.Time),
				SourceUID: new(uint),
				Type:      new(string),
				Payload:   nil,
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.m.isValid(); (err != nil) != tt.wantErr {
				t.Errorf("Message.isValid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
