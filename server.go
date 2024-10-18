package main

import (
	"context"
	"net"

	"github.com/rs/zerolog/log"
	"tailscale.com/client/tailscale"
	"tailscale.com/ipn"
	"tailscale.com/tsnet"
)

// Server represents the tsnet server and its configuration
type Server struct {
	srv         *tsnet.Server
	localClient *tailscale.LocalClient
}

// NewServer creates a new Server instance
func NewServer(hostname string, ephemeral bool, stateStore ipn.StateStore, authKey string) *Server {
	srv := &tsnet.Server{
		Hostname:  hostname,
		Ephemeral: ephemeral,
		Store:     stateStore,
		AuthKey:   authKey,
		Logf: func(format string, args ...any) {
			log.Debug().Str("module", "tailscale").Msgf(format, args...)
		},
		UserLogf: func(format string, args ...any) {
			log.Info().Str("module", "tailscale").Msgf(format, args...)
		},
	}

	return &Server{
		srv: srv,
	}
}

// Start initializes and starts the tsnet server
func (s *Server) Start() error {
	log.Info().Msg("starting tunnel")

	err := s.srv.Start()
	if err != nil {
		return err
	}

	s.localClient, err = s.srv.LocalClient()
	if err != nil {
		return err
	}

	log.Info().Msgf("tunnel is ready")
	return nil
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() {
	if err := s.localClient.Logout(context.Background()); err != nil {
		log.Error().Err(err).Msg("Failed to log out")
	}
	if err := s.srv.Close(); err != nil {
		log.Error().Err(err).Msg("Failed to close server")
	}
}

// Dial creates a new connection to the specified address
func (s *Server) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	return s.srv.Dial(ctx, network, address)
}

// Listen creates a new listener for the specified network and address
func (s *Server) Listen(network, address string) (net.Listener, error) {
	return s.srv.Listen(network, address)
}
