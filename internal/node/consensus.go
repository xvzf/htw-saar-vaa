package node

type consensusElection struct {
	firstNUID   uint          // Receiving edge for spanning-tree forwarding of echo
	M           int           // currently highest leader-election
	exploreFrom map[uint]bool // keep track of received explore
	echoFrom    map[uint]bool // keep track of received echo (when this node is supposed to be elected as leader)
	wonElection bool
}
