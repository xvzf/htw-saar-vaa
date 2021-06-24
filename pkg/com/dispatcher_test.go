package com

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDispatcher(t *testing.T) {
	listen := "0.0.0.0:1234"
	c := make(chan *Message)
	d := NewDispatcher(listen, c)
	v, ok := d.(*listenConfig)
	assert.True(t, ok, "NewDispatcher constructs *listenerConfig")
	assert.Equal(t, listen, v.listen, "listen argument is passed to config")
	assert.Equal(t, c, v.handleChan, "channel argument is passed to config")
}
