package neigh

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/awalterschulze/gographviz"
	"github.com/rs/zerolog/log"
)

// Config holds a list of UIDs and neighbors
type Config struct {
	Nodes map[uint]string // UID -> connect string (`<host>:<port>`)
}

// NeighMap contains all neighbour relationships
type NeighMap struct {
	Neighs map[uint][]uint
}

// LoadConfig reads a config file and outputs a config
func LoadConfig(path string) (*Config, error) {
	c := &Config{
		Nodes: make(map[uint]string),
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read file line by line
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := s.Text()
		// Parse line and extract UID -> connect string pair
		la := strings.Split(l, " ")
		if len(la) != 2 { // Check if the line is (seemingly) formated correctly
			return nil, errors.New("invalid line format, has to follow `<uid> <connect string>`")
		}
		uid, err := strconv.Atoi(la[0])
		conn := la[1]
		if err != nil {
			return nil, err
		} else if uid < 0 {
			return nil, errors.New("UID is not a positive integer")
		} else if !strings.Contains(conn, ":") {
			return nil, errors.New("connect string is invalid, needs to follow the expression <host>:<port>")
		}
		c.Nodes[uint(uid)] = conn
	}

	return c, nil
}

// LoadGraph reads a graph config file containing the UID<->UID pairs
func LoadGraph(path string) (*NeighMap, error) {
	nm := &NeighMap{
		Neighs: make(map[uint][]uint),
	}
	b, err := ioutil.ReadFile(path) // Read file ontent
	if err != nil {
		return nil, err
	}
	// Parse the graph using gographviz
	graphAst, err := gographviz.Parse(b)
	if err != nil {
		return nil, err
	}
	graph := gographviz.NewGraph()
	if err := gographviz.Analyse(graphAst, graph); err != nil {
		return nil, err
	}
	// Parse graph and populate NeighMap
	for _, e := range graph.Edges.Edges {
		log.Debug().Msgf("Adding edge %s --- %s", e.Dst, e.Src)
		// Extract UIDs from graph
		ai, err := strconv.Atoi(e.Dst)
		if err != nil {
			return nil, err
		} else if ai < 0 {
			return nil, errors.New("uid is not positive")
		}
		bi, err := strconv.Atoi(e.Src)
		if err != nil {
			return nil, err

		} else if bi < 0 {
			return nil, errors.New("uid is not positive")
		}
		// Add to neigh map
		var l uint
		var h uint
		if bi < ai {
			l, h = uint(bi), uint(ai)
		} else {
			h, l = uint(bi), uint(ai)
		}
		if v, ok := nm.Neighs[l]; !ok {
			nm.Neighs[l] = []uint{h}
		} else {
			nm.Neighs[l] = append(v, h)
		}
	}

	return nm, nil
}

// GenGraph generates a random communication graph
func GenGraph(path string, n, m uint) error {
	// Seed PRGN
	rand.Seed(time.Now().UTC().UnixNano())
	// Check if we're okay
	if m < n {
		return errors.New("condition m > n failed")
	} else if m > (n * (n - 1)) {
		return errors.New("condition m < n*(n-1) failed")
	}

	edges := make(map[uint][]uint)

	add := func(a, b uint) bool {
		var l uint
		var h uint

		// We're inserting low -> high to keep consistency
		if a < b {
			l, h = a, b
		} else {
			h, l = a, b
		}

		if h == l {
			return false
		}

		// Check if edge already exists
		var ok bool
		v, ok := edges[l]
		if ok {
			for _, i := range v {
				if i == h {
					return false
				}
			}
			edges[l] = append(edges[l], h)
		} else {
			edges[l] = []uint{h}
		}

		return true
	}

	elen := func(edges map[uint][]uint) uint {
		total := 0
		for _, v := range edges {
			total += len(v)
		}
		return uint(total)
	}

	var i uint
	for i = 1; i <= n; i++ { // 1 ... n
		for {
			j := 1 + rand.Intn(int(n))
			if add(i, uint(j)) {
				break
			}
		}
	}
	for elen(edges) != m { // fill up edges until m is reached
		a := 1 + rand.Intn(int(n))
		b := 1 + rand.Intn(int(n))
		add(uint(a), uint(b))
	}

	// Build graph
	graphAst, _ := gographviz.ParseString(`graph G {}`)
	graph := gographviz.NewGraph()
	if err := gographviz.Analyse(graphAst, graph); err != nil {
		return err
	}

	// Add nodes
	for i = 1; i <= n; i++ { // 1 ... n
		if err := graph.AddNode("G", fmt.Sprint(i), nil); err != nil {
			return err
		}
	}
	// Add edges without attributes
	for k, v := range edges {
		log.Info().Msgf("%s <-> %s", fmt.Sprint(k), fmt.Sprint(v))
		for _, e := range v {
			if err := graph.AddEdge(fmt.Sprint(k), fmt.Sprint(e), false, nil); err != nil {
				return err
			}
		}
	}

	dot := graph.String()
	return ioutil.WriteFile(path, []byte(dot), 0644)
}
