package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/niranjan94/tsnet-relay"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"tailscale.com/ipn/store/mem"
)

// Global variables
var (
	srv *tsnet_relay.Server

	hostname           string
	tags               string
	ephemeral          bool
	stateStoreName     string
	configPath         string
	verbose            bool
	idleTimeoutSeconds int
)

func main() {
	// Parse command-line flags
	flag.StringVar(&hostname, "hostname", "", "Hostname to use for the server")
	flag.StringVar(&tags, "advertise-tags", "", "Tags to use for the server")
	flag.BoolVar(&ephemeral, "ephemeral", false, "Use an ephemeral hostname")
	flag.StringVar(&stateStoreName, "state", "mem:", "State store to use for the server")
	flag.StringVar(&configPath, "config", "config.json", "Path to the configuration file")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.IntVar(&idleTimeoutSeconds, "idle-timeout", 0, "Exit after specified number of seconds with no incoming connections (0 to disable)")
	flag.Parse()

	// Default level for this example is info, unless debug flag is present
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	if hostname == "" {
		log.Fatal().Msg("Hostname is required")
	}

	// Configure zerolog
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	// Create state store
	stateStore, err := mem.New(func(format string, args ...any) {
		log.Info().Str("module", "tailscale-state").Msgf(format, args...)
	}, stateStoreName)
	if err != nil || stateStore == nil {
		log.Fatal().Err(err).Msg("Failed to create state store")
	}

	log.Info().Msgf("using state store store: %v", stateStore)

	// Resolve auth key
	log.Info().Msg("resolving auth key")
	authKeyFromEnv := os.Getenv("TS_AUTHKEY")
	authKey, err := tsnet_relay.ResolveAuthKey(context.Background(), authKeyFromEnv, tags)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to resolve auth key")
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the server
	srv = tsnet_relay.NewServer(hostname, ephemeral, stateStore, authKey)
	if err := srv.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}

	// Load initial configuration
	config, err := tsnet_relay.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Set up initial tunnels
	if err := srv.SetupTunnels(ctx, config.Tunnels); err != nil {
		log.Fatal().Err(err).Msg("Failed to set up initial tunnels")
	}

	// Start the idle timeout checker
	srv.StartIdleTimeoutChecker(ctx, time.Duration(idleTimeoutSeconds)*time.Second)

	// Set up signal handling
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	log.Info().Int("pid", os.Getpid()).Msg("send SIGUSR1 to reload configuration")
	// Handle signals
	for {
		sig := <-sigs
		switch sig {
		case syscall.SIGUSR1:
			log.Info().Msg("Received SIGUSR1. Reloading configuration.")
			if _, err := tsnet_relay.ReloadConfig(ctx, configPath, srv, config); err != nil {
				log.Error().Err(err).Msg("Failed to reload configuration")
			}
		case syscall.SIGINT, syscall.SIGTERM:
			log.Info().Msg("Shutting down...")
			cancel()
			srv.Shutdown()
			log.Info().Msg("Shutdown complete")
			return
		}
	}
}
