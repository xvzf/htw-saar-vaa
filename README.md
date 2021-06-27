# Verteilte Algorithment und Anwendungen (vaa)
> This project is based on a distributed systems course at my university, tailed towards bringing better understandings
> for communication and state in those.

TODO:
- [ ] Rumor experiment; 20 iterations with different n, m, c

## Starting up multiple nodes

The Jsonnet template `hack/gen-launch.jsonnet` allows generation of arbitrary launch scripts up to 999 nodes (afterwards there will be port collisions).
The `make gen` command generates a random graph, node configuration and a `launch.sh` which will start each node in a dedicated tmux pane for easy debugging.

The helper function `make startup` starts all nodes and initiates the communication flow, `make shutdown` stops all processes. The tmux session can be closed with `<CTRL-B>:kill-session` which removes the tedious task of clearing 10+ tmux panes.

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

## Discovery Messages
> Discovery messages are used for discovering neighbours/marking them as active

All discovery messages are of message type `DISCOVERY` and carry the operation as payload.

Supported payload operations:

| Operation  | Action                                                                                                                      |
|------------|-----------------------------------------------------------------------------------------------------------------------------|
| `HELLO`    | Transmits own UID to a neighbor node, allowing them to register it as active, initiated with the `STARTUP` control command  |


## Distributed Consensus Messages
> Experimenting with leader-election in unknown network structures, distributed agreement on values

All discovery messages are of message type `CONSENSUS` and carry the operation as payload.

Supported payload operations:

| Operation              | Action                                                                                 |
|------------------------|----------------------------------------------------------------------------------------|
| `explore <node-id>`    | Part of leader-election with echo-based algorithm (see Experiments for further detail) |
| `echo <node-id>`       | Part of leader-election with echo-based algorithm (see Experiments for further detail) |
| `echo <node-id>`       | Part of leader-election with echo-based algorithm (see Experiments for further detail) |



## Control Messages
> Control messages are only for `client -> cluster node` communication.

All control messages are of message type `CONTROL` and carry the operation as payload.

Supported payload operations:

| Operation  | Action                                                                        |
|------------|-------------------------------------------------------------------------------|
| `STARTUP`  | triggers startup, the node registers to neighbors                             |
| `SHUTDOWN` | triggers graceful shutdown of the node. Remaining transactions are finalised. |
| `DISTRIBUTE <TYPE> <PAYLOAD>` | this leads to a node sending the payload to all neighbours |

The client can be used to execute control commands, e.g.:
```
go run cmd/client/main.go --type="CONTROL" --payload="SHUTDOWN" --connect "127.0.0.1:4000"
```
connects to the node running on localhost port 4000

## Experiments

### Rumor distribution
All rumors are of message type `RUMOR`.

One node receives a rumor following the payload `<C>;<CONTENT>` where `C` indicates the threshold at which the node accepts the rumor (e.g. `C = 2` leads to a node trusting the rumor when it receives it from 2 neighbors).
Once a node receives an unknown rumor, it propagates it to all neighbours but the sending one. If it is known it just increases the internal counter.

The initial rumor is started via the control message, e.g. `DISTRIBUTE RUMOR 2;rumor2trust`.


### Distributed Concensus

#### Scenario

##### Election

A random number of nodes starts a distributed election based on an echo algorithm. The winner will start the distributed concensus
An `explore <node-id>` is send to all edges, once a node received an `expore <node-id>` on all edges, it sends an `echo <node-id>` to the initial receiving edge (this effectively feedbacks the echo across the spanning tree of the distributed network).
All nodes contain an internal counter (`M`) which is on startup set to `-inf`. `<node-id> > M` overrides `M` and stops the propagation of either *echo* or *explore*. 

##### Concensus

Multiple (`n`) nodes try to align on a common value, without centralised entity. There are `1-m` valuesa are available.
On concensus start, every node (`P_k`) selects one value as its favourite.
During the election, nodes communicate only in pairs based on their communication graph.
