package node

import "github.com/xvzf/vaa/pkg/com"

type Extension interface {
	// Hanlde handles messages of a specific type
	Handle(h *handler, msg *com.Message) error
}
