package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
	"github.com/xvzf/vaa/pkg/neigh"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	// debug := flag.Bool("debug", true, "enable debug mode")
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
}

func main() {
	config := flag.String("config", "", "path to config file")
	uid := flag.Uint("uid", 0, "UID to set for originating request")
	t := flag.String("type", "CONTROL", "message type")
	connect := flag.String("connect", "127.0.0.1:4000", "target node")
	p := flag.String("payload", "STARTUP", "message payload")

	flag.Parse()
	log.Info().Msgf("Loading configuration from file %s", *config)

	// Construct message
	msg := com.Msg(*uid, *t, *p)

	if *config != "" {
		// Send requests to all nodes
		c, err := neigh.LoadConfig(*config)
		if err != nil {
			log.Err(err).Msg("Failed loading config")
			return
		}
		for _, netaddr := range c.Nodes {
			if err := com.Send(netaddr, msg); err != nil {
				log.Err(err).Msg("Request failed")
			}
		}
	} else {
		// Send request to single node
		if err := com.Send(*connect, msg); err != nil {
			log.Err(err).Msg("Request failed")
		}
	}
}
