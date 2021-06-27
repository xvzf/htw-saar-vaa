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

| Operation                | Action                                                                                 |
|--------------------------|----------------------------------------------------------------------------------------|
| `coordinator`            | Triggers leader-election                                                               |
| `explore;<node-id>`      | Part of leader-election with echo-based algorithm (see Experiments for further detail) |
| `child;<node-id>;<0|1>`  | Part of leader-election with echo-based algorithm (see Experiments for further detail) |
| `echo;<node-id>`         | Part of leader-election with echo-based algorithm (see Experiments for further detail) |
| `leader;<node-id>`       | Part of leader-election with echo-based algorithm (see Experiments for further detail) |



## Control Messages
> Control messages are only for `client -> cluster node` communication.

All control messages are of message type `CONTROL` and carry the operation as payload.

Supported payload operations:

| Operation                     | Action                                                                        |
|-------------------------------|-------------------------------------------------------------------------------|
| `STARTUP`                     | triggers startup, the node registers to neighbors                             |
| `SHUTDOWN`                    | triggers graceful shutdown of the node. Remaining transactions are finalised. |
| `DISTRIBUTE <TYPE> <PAYLOAD>` | this leads to a node sending the payload to all neighbours                    |

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
> The election algorithm used here is a derivate of the wave algorithm. The implementation is done in `internal/node/consensus.go`

A random number of nodes starts a distributed election based on an echo algorithm. The winner will start the distributed concensus.
The largest `node-id` should win.

Per `node-id`, each node keeps track on how many explores have been received, how many echos as well as what neighbours are its childs in a spanning tree.

To do this, a node initiates the leader election by sending an `explore <node-id>` to all neighbors and sets its internal highest seen `node-id` (further referenced as `m`) to its own.
Once a node receives the `explore <node-id>` message the first time, it stores the sender UID as `srcUID` (parent in spanning tree) responds with `child <node-id>;1` to the node with the uid `srcUID` and sets the `received_explore` counter to 1. In case the `explore <node-id>` was already seen, it sends the message `child <node-id>;0` and increase the internal `received_explore` counter.

Once a node received a `child <node-id>;<1|0>`, it increases the internal counter `received_parentmsg` and in case the response is `1` it also stores the `SourceID` of the message in the internal array `child_uids`. This array is later used to propagate messages along the spanning tree.

Once a node receives an `echo <node-id>`, it increases the internal counter `received_echo`.

The following conditions lead a node to send an `echo <node-id>` to the its node with the UID `srcUID` (first node sending the explore for the `node-id`):
- `(neigh_count == received_parentmsg) && len(child_uids) == received_echo` (all childs reported an echo)
- `(neigh_count == received_parentmsg) && len(child_uids) == 0 && received_expore == len(neighbours)` (the node is a leaf in the spanning-tree)

The node that initiated the leader election wins when it receives an `echo <node-id>` where `node-id` is its own ID.
Afterwards the leader propagates the result (`leader <node-id>`) to all nodes with their UID being in `child_uids`.
Once a node received the message, it drops `echo` and `explore` messages from now on and forwards the `leader;<node-id>` to its own childs (nodes with their uid in `child_uids`).

The now constructed, distributed spanning tree can later be used for additional communication of control messages.

##### Concensus

Multiple (`n`) nodes try to align on a common value, without centralised entity. There are `1-m` valuesa are available.
On concensus start, every node (`P_k`) selects one value as its favourite.
During the election, nodes communicate only in pairs based on their communication graph.
