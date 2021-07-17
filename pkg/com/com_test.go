package com

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

// Test communication between server & client
func TestCommunication_valid(t *testing.T) {
	net_addr := "127.0.0.1:33333"
	test_msg := &Message{
		UUID:      StrPointer(uuid.NewString()),
		Timestamp: timePointer(time.Now().UTC()),
		SourceUID: uintPointer(2),
		Type:      StrPointer("CONTROL"),
		Payload:   StrPointer("some payload 1234"),
	}
	recv_chan := make(chan *Message)
	fmt.Print(recv_chan)

	defer close(recv_chan)
	ctx, cancel := context.WithCancel(context.Background())
	exitCtx, dispatchCancel := context.WithCancel(context.Background())

	// Construct dispatcher
	d := NewDispatcher(net_addr, recv_chan)

	go func() {
		// Make sure we handle the go routine properly as well
		defer dispatchCancel()
		// Start server
		err := d.Run(ctx)
		assert.Nil(t, err, "Should be nil")
	}()

	// allow some time for the server to startup
	time.Sleep(100 * time.Millisecond)

	// Send a message
	Send(net_addr, test_msg)

	// Receive message
	recv_msg := <-recv_chan

	// Compare if message content has been passed
	assert.Equal(t, *test_msg.UUID, *recv_msg.UUID)
	assert.Equal(t, *test_msg.Timestamp, *recv_msg.Timestamp)
	assert.Equal(t, *test_msg.SourceUID, *recv_msg.SourceUID)
	assert.Equal(t, *test_msg.Type, *recv_msg.Type)
	assert.Equal(t, *test_msg.Payload, *recv_msg.Payload)
	log.Info().Msg("Compared msg")

	// Close context & wait for dispatcher to exit
	cancel()
	fmt.Print("cancel")
	<-exitCtx.Done()
	fmt.Print("exitchan")
}

// Test invalid listen address
func TestCommunication_invalidListener(t *testing.T) {
	net_addr := "somethinginvalid:-124-"
	recv_chan := make(chan *Message)
	defer close(recv_chan)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Construct dispatcher
	d := NewDispatcher(net_addr, recv_chan)

	err := d.Run(ctx)
	assert.NotNil(t, err, "Invalid listener address provided, should exit")
}
