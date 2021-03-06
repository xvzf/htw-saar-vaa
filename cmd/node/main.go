package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/internal/node"
	"github.com/xvzf/vaa/pkg/com"
	"github.com/xvzf/vaa/pkg/neigh"
)

func init() {
	// Setup logging
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

}

func main() {
	var wg sync.WaitGroup
	var neighs *neigh.Neighs
	var err error

	debug := flag.Bool("debug", false, "enable debug mode")

	config := flag.String("config", "./config", "path to config file")
	graph := flag.String("graph", "", "path to graph")
	uid := flag.Uint("uid", 1, "Node UID")
	metric := flag.String("metric", ":9111", "metric endpoint")

	consensusM := flag.Int("consensus-m", 5, "number of discrete timestamps")
	consensusAmax := flag.Int("consensus-amax", 3, "max number of voting rounds")
	consensusP := flag.Int("consensus-p", 2, "How many random neighbours to choose")
	consensusS := flag.Int("consensus-s", 3, "How many nodes are asked to initiate the voting process")

	flag.Parse()

	// Debug logs
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	// Start metric server
	log.Info().Msgf("Starting metric endpoint at %s/metrics", *metric)
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(*metric, nil); err != nil {
			log.Err(err).Msg("Failed serving metrics endpoint")
		}
	}()

	// Load configuration / construct neighbors for thise node
	c, err := neigh.LoadConfig(*config)
	if err != nil {
		log.Err(err).Msg("Failed to load configuration")
		return
	}
	netAddr, ok := c.Nodes[*uid]
	if !ok {
		log.Error().Msg("UID not in config")
		return
	}
	netAddrSplit := strings.Split(netAddr, ":")
	if len(netAddrSplit) != 2 {
		log.Error().Msg("UID network address invalid, has to follow <host>:<port>")
		return
	}
	listen := fmt.Sprintf(":%s", netAddrSplit[1])

	if *graph != "" {
		log.Info().Msgf("Loading node config from configuration file + communication graph")
		neighs, err = neigh.NeighsFromConfigAndGraph(*uid, *config, *graph)
	} else {
		log.Info().Msgf("Loading node config from configuration file")
		neighs, err = neigh.NeighsFromConfig(*config)
	}
	if err != nil {
		log.Err(err).Msg("Failed to load configuration")
		return
	}

	log.Info().Msgf("Loaded configuration for UID %d", *uid)

	// Communication channels + Dispatcher
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	recvChan := make(chan *com.Message, 1) // buffer but make sure we're still FIFO
	d := com.NewDispatcher(listen, recvChan)
	n := node.New(*uid, cancelCtx, neighs)

	// Register node extensions
	n.Register(node.NewControlExtension())
	n.Register(node.NewDiscoveryExtension())
	n.Register(node.NewRumorExtension()) // Rumor experiment
	n.Register(node.NewDistributedBankingExtension())
	n.Register(node.NewConsensusExtension(*consensusS, *consensusM, *consensusP, *consensusAmax)) // Consensus experiment

	// Start message dispatcher (aka receiver)
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := d.Run(ctx)
		if err != nil {
			log.Err(err)
		}
	}()

	// Start message handler (aka node)
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := n.Run(ctx, recvChan)
		if err != nil {
			log.Err(err)
		}
	}()

	// Handle ctrl + c
	osc := make(chan os.Signal, 1)
	signal.Notify(osc, os.Interrupt, syscall.SIGTERM)
	select {
	case <-osc:
		log.Info().Msg("Received CTRL+C, shutting down")
		cancelCtx()
	case <-ctx.Done():
		log.Info().Msg("Control message triggered shutdown")
	}
	wg.Wait() // Wait for listeners/handlers to shutdown
	log.Info().Msg("ByeBye")
}
