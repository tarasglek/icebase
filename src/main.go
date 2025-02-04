package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"
)

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	postEndpoint := flag.String("post", "", "send POST request to specified endpoint e.g.: echo 'select now()' | ./icebase -post /query")
	querySplitting := flag.Bool("query-splitting", false, "enable semicolon query splitting")
	logLevel := flag.String("log-level", "info", "set the logging level (debug, info, warn, error); can also be set via LOG_LEVEL env var")
	versionFlag := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("Version:", Version)
		return
	}

	// Initialize logging
	InitLogger(*logLevel)

	opts := []IceBaseOption{}
	if *querySplitting {
		opts = append(opts, WithQuerySplittingEnabled())
	}

	ib, err := NewIceBase(opts...)
	if err != nil {
		log.Fatal().Msgf("Failed to initialize IceBase: %v", err)
	}
	defer ib.Close()

	// If -post flag is provided, act as CLI client
	if *postEndpoint != "" {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal().Msgf("Failed to read stdin: %v", err)
		}

		jsonResponse, err := ib.PostEndpoint(*postEndpoint, string(input))
		if err != nil {
			log.Fatal().Msgf("POST request failed: %v", err)
		}

		fmt.Println(jsonResponse)
		return
	}

	// Start server
	addr := fmt.Sprintf(":%d", *port)
	log.Info().Msgf("Starting server on %s", addr)
	handler := ib.RequestHandler()
	http.HandleFunc("/query", handler)
	http.HandleFunc("/parse", handler)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Error().Msgf("Error starting server: %v", err)
		flag.Usage()
		os.Exit(1)
	}
}
