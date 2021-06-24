package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/com"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	// debug := flag.Bool("debug", true, "enable debug mode")
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
}

func main() {
	config := flag.String("config", "./config", "path to config file")
	log.Info().Msgf("Loading configuration from file %s", *config)

	connect := flag.String("connect", "127.0.0.1:4000", "target node")

	// Message related
	uid := flag.Uint("uid", 0, "UID to set for originating request")
	t := flag.String("type", "CONTROL", "message type")
	p := flag.String("payload", "STARTUP", "message payload")

	flag.Parse()

	// Construct message
	msg := com.Msg(*uid, *t, *p)

	// Send request
	if err := com.Send(*connect, msg); err != nil {
		log.Err(err).Msg("Request failed")
	}
}
