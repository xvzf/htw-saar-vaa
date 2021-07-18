# Verteilte Algorithment und Anwendungen (vaa)
> This project is based on a distributed systems course at my university, tailed towards bringing better understandings
> for communication and state in them.


## Starting up multiple nodes
> Note: Experiments showed everything > ~50 nodes runs into network timeouts

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

## Implementation

### Node
> The node process is defined in `cmd/node.go`

The base of the experiments are a very lightweight `Node` implementation allowing the registration of multiple extensions, routing a certain message type, e.g. `CONTROL` to a specified extension. Moreover, extensions support *preflight* initiation hooks for e.g. for starting a watcher process.
Each node reads the configuration file and the graph and constructs a `Handler` (containing all node *business logic*), containing information about its own identifier and the neighbour nodes.
It also contains all registered extensions

```go
type handler struct {
	uid    uint
	neighs *neigh.Neighs
	exit   context.CancelFunc
	wg     sync.WaitGroup
	ext    map[string]Extension
}
```

The extension interface is very simple and provides a message handler & the preflight hook. Latter one is passed a context as well allowing graceful termination. For registering an extension, the extension alongside the message prefix is passed, e.g. `h.Register(ext, "PREFIX")`.
```go
type Extension interface {
	// Hanlde handles messages of a specific type
	Handle(h *handler, msg *com.Message) error
	// Preflight initialised additional goroutines/communication paths
	Preflight(ctx context.Context, h *handler) error
}
```

Each extension just implements the interface and maintains full control for parameters, e.g. in a later experiment, custom variables are passed to the extension before it is added to the node handler.

The communication is encapsulated from the handler logic and is implemented in `pkg/com/dispatcher.go` (server) and `pkg/com/client.go` (client).
The server handles all incoming connections in a sequential order (:warning: this is required for the FIFO assumptions in some of the algorithms but sometimes leads to timeouts). After the message is valid and decoded, it is sent to a [go channel](https://tour.golang.org/concurrency/2) which the handler listens on.
This setup prevents deadlocks from a node not responding to messages while it is still processing one while keeping the FIFO order in place.

Both the node handler and the dispatcher are context aware and terminate gracefully - either on a stop signal coming from the operating system or a control message on the network.

### Client
> Client implemented in `cmd/client.go`

The client for the network is very simple and is only used for launching certain experiments or sending control messages.
It reads the same configuration as the node and takes the assumption all nodes can be reached. Requests are sent in parallel which allows e.g. multiple nodes to start the leader election at the same time; with just distributing messages across the network there will always be one node reached first and possibly winning the election.

### Graph Generation
> Graph generation implemented in `cmd/graphgen.go`

The graph generation is rather simple, it uses a [community package](https://pkg.go.dev/github.com/awalterschulze/gographviz@v2.0.3+incompatible) to parse the Graphviz file format and to interact with graphs.
When a random graph with `n` nodes and `e` edges shall be generated, edges are randomly inserted (unique) until the number of edges matches. Each node is assigned a minimum of `1` edge therefore eliminating the risk of generating unconnected sub-graphs.


## Discovery Messages
> Discovery messages are used for discovering neighbours/marking them as active

All discovery messages are of message type `DISCOVERY` and carry the operation as payload.

Supported payload operations:

| Operation  | Action                                                                                                                      |
|------------|-----------------------------------------------------------------------------------------------------------------------------|
| `HELLO`    | Transmits own UID to a neighbor node, allowing them to register it as active, initiated with the `STARTUP` control command  |

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


## Distributed Consensus Messages
> Experimenting with leader-election in unknown network structures, distributed agreement on values

All discovery messages are of message type `CONSENSUS` and carry the operation as payload.

Supported payload operations:

> Leader election

| Operation                                     | Action                                                                                 |
|-----------------------------------------------|----------------------------------------------------------------------------------------|
| `coordinator`                                 | Triggers leader-election                                                               |
| `explore;<node-id>`                           | Part of leader-election with echo-based algorithm (see Experiments for further detail) |
| `child;<node-id>;<0\|1>`                      | Part of leader-election with echo-based algorithm (see Experiments for further detail) |
| `echo;<node-id>`                              | Part of leader-election with echo-based algorithm (see Experiments for further detail) |
| `leader;<node-id>`                            | Part of leader-election with echo-based algorithm (see Experiments for further detail) |

> Double Counting (Termination)

| Operation                                                 | Action                                                                                 |
|-----------------------------------------------------------|----------------------------------------------------------------------------------------|
| `stateRequest;uid`                                        | Part of double counting with echo-based algorithm (see Experiments for further detail) |
| `stateResponse;uid;<t\|f>;<count_msg_in>;<count_msg_out>` | Part of double counting with echo-based algorithm (see Experiments for further detail) |

> Collection/Voting

| Operation                                     | Action                                                                                   |
|-----------------------------------------------|------------------------------------------------------------------------------------------|
| `collectRequest;uid`                          | Part of result collection with echo-based algorithm (see Experiments for further detail) |
| `collect;uid;<t\|f>;timestamp`                | Part of result collection with echo-based algorithm (see Experiments for further detail) |
| `proposal;timestamp`                          | Propose time to another node                                                             |
| `proposalResponse;timestamp`                  | Align on the in-between time between two processes                                       |
| `voteBegin`                                   | Initiate vote request                                                                    |

## Banking Messages
> Experimenting with mutual exclusion (unknown network structures), consistent snapshot

All discovery messages are of message type `BANKING` and carry the operation as payload.

Supported payload operations:

> Leader election (same as in Distributed Consensus)

| Operation                                     | Action                                                                                 |
|-----------------------------------------------|----------------------------------------------------------------------------------------|
| `coordinator`                                 | Triggers leader-election                                                               |
| `explore;<node-id>`                           | Part of leader-election with echo-based algorithm (see Experiments for further detail) |
| `child;<node-id>;<0\|1>`                      | Part of leader-election with echo-based algorithm (see Experiments for further detail) |
| `echo;<node-id>`                              | Part of leader-election with echo-based algorithm (see Experiments for further detail) |
| `leader;<node-id>`                            | Part of leader-election with echo-based algorithm (see Experiments for further detail) |

> Lamport Mutual Exclusion

| Operation                                                | Action                                                                                 |
|----------------------------------------------------------|----------------------------------------------------------------------------------------|
| `lockRequest;<timestamp>;<node-id>;<req-timestamp>`      | Part of the mutual exclusing lock based on the lamportMutex                            |
| `lockAck;<timestamp>;<node-id>;<req-timestamp>`          | Part of the mutual exclusing lock based on the lamportMutex                            |
| `lockRelease;<timestamp>;<node-id>;<req-timestamp>`      | Part of the mutual exclusing lock based on the lamportMutex                            |

> Transactions, those operations are distributed via flooding

| Operation                                                            | Action                                                                                   |
|----------------------------------------------------------------------|------------------------------------------------------------------------------------------|
| `transactStart;<timestamp>;<uid>;<node-id-pj>;<balance>;<p>`         | Start transaction, transmit balance & p so `P_j` can perform its check; `<target-id>`    |
| `transactAck;<timestamp>;<uid>`                                      | `P_j` acknowledges it performed the action                                               |
| `transactGetBalance;<timestamp>;<uid>;<node-id-pj>;`                 | Get the balance of the target-node                                                       |
| `transactBalance;<timestamp>;<uid>;<balance>`                        | Balance of `P_j`; as with other messages, this is mutual exclusive -> ID is not required |

> Consistent snapshot (Chandy Lamport)

| Operation                                                            | Action                                                                                   |
|----------------------------------------------------------------------|------------------------------------------------------------------------------------------|
| `marker;<uid>`                                                       | Starts the snapshot collection following the Chandy Lamport algorithm                    |
| `state;<uid>;<base64compressedjsonstate>`                            | Feedbacks the state after closing the snapshot to the coordinator                        |

## Experiments

### Rumor distribution
All rumors are of message type `RUMOR`.

One node receives a rumor following the payload `<C>;<CONTENT>` where `C` indicates the threshold at which the node accepts the rumor (e.g. `C = 2` leads to a node trusting the rumor when it receives it from 2 neighbors).
Once a node receives an unknown rumor, it propagates it to all neighbours but the sending one. If it is known it just increases the internal counter.

The initial rumor is started via the control message, e.g. `DISTRIBUTE RUMOR 2;rumor2trust`.


### Distributed Concensus

#### Scenario

##### Election
> The election algorithm used here is a derivate of the wave algorithm. The implementation is done in `internal/node/leader.go`
> The election process has been encapsulated into its own module/extension and will be re-used in different scenarios

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

> The resulting spanning tree allows efficient *flooding* of messages across the spanning tree.

*Notes*:
- The election algorithm seems very robust in networks with up to 50 nodes
- Sometimes, network timeouts cause a deadlock, this happens only in larger networks
- The spanning tree is effective for reducing the number of messages when propagating a message
- The spanning tree propagation removes the need for having an identifier on each message to keep track of *known* messages; it always terminates and visits each node just once.


##### Concensus

Multiple (`n`) nodes try to align on a common value, without centralised entity. There are `1-m` valuesa are available.
On concensus start, every node (`P_k`) selects one value as its favourite.
During the election, nodes communicate only in pairs based on their communication graph.

> Voting
After a coordinator initiated the voting process using the `vote` message, each node performs the alignment until the `aMax` condition is met. Afterwards, messages are dropped.

> Termination based on Double Counting Method

In order to allow termination detection, all nodes keep track of incoming/outgoing messages for the voting process.
The elected coordinator regulary checks the termination state using the double counting message.
The coordinator sends a request along the spanning tree asking to return the state of the network.
Receiving nodes propagate the request to its childs and wait for a response of those.
Incoming/Outgoing message counters are summed up.
The passive/active state is represented in the first part of the `state` response and is *ORed* for all childs and the receiving node before propagating.
This allows to make sure all network nodes are in a passive state; e.g. `node_1_active('false') && node_2_active('false') && node_3_active('true') = true`.

> Collecting results

Results are - as all control messages after the leader election, transfered across the spanning tree. a `collectRequest` is propagated along the child nodes and nodes send an accumulated `collect` response once all children have reported theirs.


### Distributed Mutual Exclusion And Consistent Snapshot
> **NOTE** for this experiment to work, there is a dependency on a successful leader election as it also constructs the spanning tree which is later on used to distribute control messages efficient across the network

#### Leader Election

The leader election uses the same implementation as the *Distributed Consensus* leader election.

#### Mutual Lock

The Lamport algorithm is used to provide a distributed lock on the graph. Since nodes (`n`) are not interconnected with each other but in a spanning tree, the message complexity increases to (worst case):
- `(n_forwards-1) * (n_request-1)`
- `(n_forwards-1) * (n_ack-1)`
- `(n_forwards-1) * (n_release-1)`
Resulting in a total message count of `3*(n-1)^2`. The complexity in distributed networks is therefore in the complexity class `O(n^2)` rather than `O(n)` when all nodes can reach each other.

A node can request a critical section execution by sending a request with its current lamport clock timestamp to all nodes in the network (propagate across the spanning tree)
Each node keeps a priority key of `(lamport_clock_timestamp, node_id)` and allows always the node with the lowest `lamport_clock_timestamp` to enter the critical section.
This node only enters the critical section when it received an acknowledgement from all other nodes in the network.
After exiting the critical section, it distributes a release message across the spanning tree, allowing the next critical section.

#### Transactions
> :warning: This experiment is by design not using efficient communication and uses flooding

Each node (`P_i`) has a local balance `B_i` (initialised on startup with a random positive value between 0 and 100k) and tries to do a transaction with `P_j`. In order to do so, the following steps are followed:
- select random target node `P_j` (*note: this can be any node in the network, not just one of the neighbours*)
- select random percentage value (`p`)
- acquire lock (*Mutual Lock* provides further details)
- send own balance value `B_i` to `P_j` along with the percentage `p`
- ask `P_j` what its balance is

on `P_i`, when receiving the message do the following:
- `B_j >= B_i` **increase own balance with `p` percent of `B_j`
- `B_j < B_i` **increase own balance with `p` percent of `B_i`

on `P_j`, when receiving the message do the following:
- `B_i >= B_j` **increase own balance with `p` percent of `B_i`
- `B_i < B_j` **increase own balance with `p` percent of `B_j`
- Send acknowledgement

The messages exchanged for this behaviour are described in the *Banking Messages* section.


#### Consistent Snapshot

The consistent snapshot is implemented using the Chandy Lamport algorithm.
When a node initates the snapshot collection (for this purpose a collector is elected, as noted in the chapter introduction, the resulting spanning tree is used for control message communication).

On initiation, the node stores its own state `S_i`; afterwards it sends a marker message to all neighbours.
At the same time, the node starts recording all incoming messages from each neighbour `C_ji`

When receiving a marker, the node checks if the marker already existed. If not, it persists its state and starts monitoring all incoming channels. The receiving edge buffer is marked as empty and no more messages are recorded here (-> mark as complete).
Afterwards the marker is sent to all outgoing edges.
When the marker is already known, the receiving edge buffer is marked as consistend and no more messages will be recorded on it.
After all receiving markers are closed, the state is send to the coordinator following the spanning tree.

Each marker is annotated with a unique identifier so multiple or even parralel consecutive queries are possible,

The coordinator checks if there are any messages in the snapshot affecting the balance of each node. If there are none, it simply adds-up the balances to get a global view. A future implementation could re-construct the time-stream of messages and simulate each node in order to also consider in-flight requests.


#### Conclusions
> This section also discusses chances of a deadlock

The implementation in this scenario is rather complex and very hard to debug. Especially when the number of nodes or edges in the network increases, the number of messages increases significantly.

With the assumptions:
- no message lost
- no node crashes
- FIFO of messages
- Every Lamport Clock timestamp is unique
a deadlock cannot happen.

However, the initial messages of the transaction experiment are sent from each node after a random interval between 0 and 3000ms. This *could* generate a possible condition where the lamport clock of two independend outgoing messages is the same and therefore the priority queue implemented in this solution, which is only based on the lamport clock timestamp, stops working as the network state of the priority queue contains two different entries for one timestamp thus none of the two nodes will receive an acknowledgement from all nodes.
A possible workaround would be to take in the `node_id` as a 2nd parameter in the priority queue to not stop the decision making.
Another option would be to add a timeout to the mutual exclusion lock which after a certain amount of time floods the a release message despite not entering the critical section (thus skipping the critical section execution).


## Benchmark results
> All benchmarks ran with a custom script on a Kubernetes cluster, Grafana Loki was used to retrieve the data afterwards

### Rumor experiment
> The whole benchmark runs ~30 minutes

Loki queries used:
```
# trusted
sum by (n, m, c) (count_over_time({namespace="default", instance=~"node-.+-.+"} |= "Now trusted" |= "$rumor_marker" [1h]))

# total_messages
sum by (n, m, c) (count_over_time({namespace="default", instance=~"node-.+-.+"} |= ">>>" [1h]))
```

| C | m (edges) | n (nodes) | trusted (nodes) | total messages |
|---|-----------|-----------|-----------------|----------------|
| 2 | 9         | 6         | 5               | 15             |
| 3 | 9         | 6         | 3               | 13             |
| 2 | 12        | 8         | 7               | 21             |
| 3 | 12        | 8         | 3               | 21             |
| 2 | 15        | 10        | 7               | 21             |
| 3 | 15        | 10        | 4               | 21             |
| 2 | 18        | 12        | 6               | 23             |
| 3 | 18        | 12        | 3               | 25             |
| 2 | 21        | 14        | 10              | 31             |
| 3 | 21        | 14        | 3               | 29             |
| 2 | 24        | 16        | 9               | 33             |
| 3 | 24        | 16        | 7               | 33             |
| 2 | 27        | 18        | 10              | 37             |
| 3 | 27        | 18        | 7               | 37             |
| 2 | 30        | 20        | 12              | 41             |
| 3 | 30        | 20        | 2               | 30             |
| 2 | 33        | 22        | 12              | 45             |
| 3 | 33        | 22        | 1               | 27             |
| 2 | 36        | 24        | 14              | 49             |
| 3 | 36        | 24        | 6               | 49             |

To sum-up the results:
- With a grown number of edges for a node, the probability grows for the node to trust the rumor (expected as more possibilities to receive the rumor)
- more nodes are not corresponding to a better trust; just the number of interconnects does
- (obvious) with a growing `c`, the probability declines a node will trust a rumor

### Consensus experiment
> Graph pictures are located at hack/images

All 10 runs are started at the same time. In order to reduce the load and allow the logging system to keep track on what's happening (aka building a graph later on), a delay has been injected for each outgoing message after the leader election

The overall results show:
- increasing the number of nodes, increases the message complexity roughly exponential
- the larger p, the more complex messages are, however it is more likely to get a common result
- propability for a common result grows with `aMax` (having `aMax` at infinite, there would always be a common result)
- (potential) the larger `s` is, the more likely it is for a successful vote

#### Multiple values for all parameters

![graph0](https://github.com/xvzf/vaa/raw/main/hack/images/graph0.png "Graph 0")

This graphs shows the increasing message complexity with growing `aMax` and number of edges in the network.
Also, it shows the agreement is more likely to happen with a smaller `m` than with a larger `m`

#### Average network activity for a similar scenario

![graph1](https://github.com/xvzf/vaa/raw/main/hack/images/graph1.png "Graph 1")

This graph shows 10 test cases running with the same settings. It shows how likely the execution happens on multiple runs.
