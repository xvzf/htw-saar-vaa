package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/internal/node"
	"github.com/xvzf/vaa/pkg/com"
)

func init() {
	// Setup logging
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	// debug := flag.Bool("debug", true, "enable debug mode")
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	// Start metric server
	metric := flag.String("metric", ":9111", "metric endpoint")
	log.Info().Msgf("Starting metric endpoint at %s/metrics", *metric)
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(*metric, nil); err != nil {
			log.Err(err).Msg("Failed serving metrics endpoint")
		}
	}()
}

func main() {
	var wg sync.WaitGroup

	config := flag.String("config", "./config", "path to config file")
	listen := flag.String("listen", ":4000", "listen port")

	flag.Parse()

	log.Info().Msgf("Loading configuration from file %s", *config)
	// Load configuration file

	// Communication channels + Dispatcher
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	recvChan := make(chan *com.Message, 1)
	d := com.NewDispatcher(*listen, recvChan)
	n := node.New(1, cancelCtx)

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
