# Verteilte Algorithment und Anwendungen (vaa)
> This project is based on a distributed systems course at my university, tailed towards bringing better understandings
> for communication and state in those.

## Communication Protocl
> The communication protocol is intended to be as simple as possible

The communication is based on unidirectional messages without returns.
A JSON object is transferred via *TCP* and the connection is immediately closed afterwards.
The message is constructed of the following keys:
```go
type Message struct {
	UUID      *string    `json:"uuid"`
	TTL       *uint      `json:"uint"`
	Timestamp *time.Time `json:"timestamp"`
	SourceUID *uint      `json:"src_uid"`
	Type      *string    `json:"type"`
	Payload   *string    `json:"payload"`
}
```
For a message to be valid, all fields need to be set. The `github.com/xvzf/vaa/pkg/com` package contains a dispatcher (aka receiving server dispatching messages to a go channel) and a client for constructing and sending messages. A message UUID is generated whenever the client is used. Sending the same message multiple times, be it to multiple targets or to the same target, the UUID changes with every transferred message. This ensures tracability and uniqueness of messages.


## Control messages
> Control messages are only for `client -> cluster node` communication.

All control messages are of message type `CONTROL` and carry the operation as payload.

Supported payload operations:

| Operation  | Action                                                                        |
|------------|-------------------------------------------------------------------------------|
| `STARTUP`  | triggers startup, the node registers to neighbors                             |
| `SHUTDOWN` | triggers graceful shutdown of the node. Remaining transactions are finalised. |


The client can be used to execute control commands, e.g.:
```
go run cmd/client/main.go --type="CONTROL" --payload="SHUTDOWN" --connect "127.0.0.1:4000"
```
connects to the node running on localhost port 4000
