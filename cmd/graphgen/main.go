package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xvzf/vaa/pkg/neigh"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	// debug := flag.Bool("debug", true, "enable debug mode")
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
}

func main() {
	graph := flag.String("graph", "./graph.txt", "path to graph definition")
	n := flag.Int("n", 6, "number of nodes")
	m := flag.Int("m", 10, "number of edges")
	create := flag.Bool("create", false, "Create or read the file")
	flag.Parse()

	if *create {
		log.Info().Msgf("Generating graph with n: %d, m: %d", *n, *m)
		err := neigh.GenGraph(*graph, uint(*n), uint(*m))
		if err != nil {
			log.Err(err).Msg("Failed to generate graph file with given arguments")
		}
		log.Info().Msgf("Stored generated graph in %s", *graph)
	} else {
		_, err := neigh.LoadGraph(*graph)
		if err != nil {
			log.Err(err).Msg("Failed to read graph file")
		}
	}
}
