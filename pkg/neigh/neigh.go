package neigh

import (
	"math/rand"
	"time"
)

// Similar to Config but only contains a subset required for this node
type Neighs struct {
	Nodes      map[uint]string // Neighbor connect addresses
	Registered map[uint]bool   // Keeps track if a neigh registered itself or not
}

// NeighsFromConfig gets neighbors
func NeighsFromConfig(config string) (*Neighs, error) {
	// Seed PRGN
	rand.Seed(time.Now().UTC().UnixNano())

	n := &Neighs{
		Nodes:      make(map[uint]string),
		Registered: make(map[uint]bool),
	}

	c, err := LoadConfig(config)
	if err != nil {
		return nil, err
	}

	maxUID := len(c.Nodes)

	for i := 0; i < 3; i++ {
		nuid := uint(1 + rand.Intn(maxUID))
		n.Nodes[nuid] = c.Nodes[nuid]
		n.Registered[nuid] = false
	}

	return nil, nil
}

// NeighsFromConfig gets neighbors
func NeighsFromConfigAndGraph(uid uint, config, graph string) (*Neighs, error) {

	n := &Neighs{
		Nodes:      make(map[uint]string),
		Registered: make(map[uint]bool),
	}

	// Load config
	c, err := LoadConfig(config)
	if err != nil {
		return nil, err
	}

	// Load neighbour map
	nm, err := LoadGraph(graph)
	if err != nil {
		return nil, err
	}

	// Extract neighbours for the node UID based on the graph
	for a, v := range nm.Neighs {
		for _, b := range v {
			if a == uid {
				n.Nodes[b] = c.Nodes[b]
				n.Registered[b] = false
			} else if b == uid {
				n.Nodes[a] = c.Nodes[a]
				n.Registered[a] = false
			}
		}
	}

	return n, nil
}
