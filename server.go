package tsnet_relay

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"tailscale.com/client/tailscale"
	"tailscale.com/ipn"
	"tailscale.com/tsnet"
)

const (
	checkInterval = 5 * time.Second
)

// Server represents the tsnet server and its configuration
type Server struct {
	srv                *tsnet.Server
	localClient        *tailscale.LocalClient
	activeTunnels      map[string]net.Listener
	activeTunnelsMutex sync.Mutex
	setupTunnelsCancel context.CancelFunc
	activeConnections  int64
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
		srv:           srv,
		activeTunnels: map[string]net.Listener{},
	}
}

// Start initializes and starts the tsnet server
func (s *Server) Start(ctx context.Context) error {
	log.Info().Msg("starting tunnel")
	status, err := s.srv.Up(ctx)
	if err != nil {
		return err
	}

	log.Info().Str("BackendState", status.BackendState).Msgf("tunnel started")
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

// incrementActiveConnections increments the active connection counter
func (s *Server) incrementActiveConnections() {
	atomic.AddInt64(&s.activeConnections, 1)
}

// decrementActiveConnections decrements the active connection counter
func (s *Server) decrementActiveConnections() {
	atomic.AddInt64(&s.activeConnections, -1)
}

// getActiveConnections returns the current number of active connections
func (s *Server) getActiveConnections() int64 {
	return atomic.LoadInt64(&s.activeConnections)
}

// SetupTunnels initializes all enabled tunnels
func (s *Server) SetupTunnels(ctx context.Context, tunnels []Tunnel) error {
	if s.setupTunnelsCancel != nil {
		s.setupTunnelsCancel()
	}

	retryContext, cancel := context.WithCancel(ctx)
	s.setupTunnelsCancel = cancel

	for _, tunnel := range tunnels {
		if tunnel.Enabled {
			log.Info().Str("name", tunnel.Name).Str("source", tunnel.Source).Str("destination", tunnel.Destination).Msg("enabling tunnel")
			go s.manageTunnel(ctx, retryContext, tunnel)
		}
	}
	return nil
}

// manageTunnel handles the lifecycle of a single tunnel
func (s *Server) manageTunnel(ctx context.Context, retryCtx context.Context, tunnel Tunnel) {
	for {
		// Check if we should exit early
		if ctx.Err() != nil || retryCtx.Err() != nil {
			return
		}

		err := s.setupTunnel(ctx, tunnel)
		if err == nil {
			// Tunnel is set up successfully, our job here is done
			return
		}

		log.Error().Err(err).Str("tunnel", tunnel.Name).Msg("Failed to set up tunnel, will retry")

		select {
		case <-retryCtx.Done():
			return
		case <-ctx.Done():
			return
		case <-time.After(checkInterval):
			// Continue to next iteration and try again
		}
	}
}

// StartIdleTimeoutChecker starts a goroutine to check for idle timeout
func (s *Server) StartIdleTimeoutChecker(ctx context.Context, timeout time.Duration) {
	if timeout == 0 {
		return // Idle timeout is disabled
	}

	log.Info().Msgf("idle timeout is set to %v seconds", timeout.Seconds())
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		lastActiveTime := time.Now()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				currentActive := s.getActiveConnections()
				if currentActive > 0 {
					lastActiveTime = time.Now()
				} else {
					idleTime := time.Since(lastActiveTime)
					if idleTime > timeout {
						log.Info().Msgf("No active connections for %v seconds. Exiting.", timeout.Seconds())
						s.Shutdown()
						os.Exit(0)
					}
				}
			}
		}
	}()
}

// setupTunnel sets up a single tunnel
func (s *Server) setupTunnel(ctx context.Context, tunnel Tunnel) error {
	sourceProto, sourceAddr, err := parseAddress(tunnel.Source)
	if err != nil {
		return err
	}

	destProto, destAddr, err := parseAddress(tunnel.Destination)
	if err != nil {
		return err
	}

	var conn net.Conn

	// Check if the destination is a Tailscale service
	if strings.HasPrefix(destProto, "tcp+tailnet") {
		// Try to connect to the destination
		conn, err = s.srv.Dial(ctx, "tcp", destAddr)
		if err != nil {
			return fmt.Errorf("destination %s is not accessible: %v", tunnel.Destination, err)
		}
	} else {
		// Try to connect to the destination
		conn, err = net.Dial(destProto, destAddr)
		if err != nil {
			return fmt.Errorf("destination %s is not accessible: %v", tunnel.Destination, err)
		}
	}

	conn.Close() // Close the connection immediately after successful dial
	log.Info().Str("destination", tunnel.Destination).Msg("Destination is accessible")

	var listener net.Listener
	if strings.HasPrefix(sourceProto, "tcp+tailnet") {
		fmt.Println("srv.Listen", sourceAddr, sourceProto)
		listener, err = s.srv.Listen("tcp", sourceAddr)
	} else {
		listener, err = net.Listen(sourceProto, sourceAddr)
	}
	if err != nil {
		return err
	}

	s.activeTunnelsMutex.Lock()
	s.activeTunnels[tunnel.Name] = listener
	s.activeTunnelsMutex.Unlock()

	log.Info().Str("name", tunnel.Name).Str("source", tunnel.Source).Str("destination", tunnel.Destination).Msg("Tunnel enabled")
	go func() {
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					if strings.Contains(err.Error(), "use of closed network connection") {
						// Listener was closed, we should exit
						return
					}
					log.Error().Err(err).Str("tunnel", tunnel.Name).Msg("Failed to accept connection")
					continue
				}
			}
			go s.handleConnection(ctx, conn, destProto, destAddr, tunnel.Primary)
		}
	}()

	return nil
}

// handleConnection manages a single connection through the tunnel
func (s *Server) handleConnection(ctx context.Context, clientConn net.Conn, destProto, destAddr string, primary bool) {
	// Only increment active connections if this is a primary connection
	if primary {
		s.incrementActiveConnections()
		defer s.decrementActiveConnections()
	}

	defer clientConn.Close()

	log.Debug().Str("client", clientConn.RemoteAddr().String()).Str("destination", destAddr).Msg("New connection")

	var remoteConn net.Conn
	var err error

	if strings.HasPrefix(destProto, "tcp+tailnet") {
		remoteConn, err = s.srv.Dial(ctx, "tcp", destAddr)
	} else {
		remoteConn, err = net.Dial(destProto, destAddr)
	}

	if err != nil {
		log.Error().Err(err).Str("destination", destAddr).Msg("Failed to connect to destination")
		return
	}
	defer remoteConn.Close()

	// Bidirectional copy
	errChan := make(chan error, 2)
	go func() {
		_, err := io.Copy(remoteConn, clientConn)
		errChan <- err
	}()
	go func() {
		_, err := io.Copy(clientConn, remoteConn)
		errChan <- err
	}()

	// Wait for either copy to finish or context to be cancelled
	select {
	case err := <-errChan:
		if err != nil {
			log.Error().Err(err).Msg("Error during data transfer")
		}
	case <-ctx.Done():
		log.Info().Msg("Connection closed due to shutdown")
	}

	log.Debug().Str("client", clientConn.RemoteAddr().String()).Msg("Connection closed")
}

// CloseTunnels closes all active tunnels
func (s *Server) CloseTunnels() {
	s.activeTunnelsMutex.Lock()
	defer s.activeTunnelsMutex.Unlock()

	for name, listener := range s.activeTunnels {
		listener.Close()
		delete(s.activeTunnels, name)
		log.Info().Str("name", name).Msg("tunnel disabled")
	}
}

// StopTunnel disables a single tunnel
func (s *Server) StopTunnel(name string) {
	s.activeTunnelsMutex.Lock()
	defer s.activeTunnelsMutex.Unlock()

	if listener, exists := s.activeTunnels[name]; exists {
		listener.Close()
		delete(s.activeTunnels, name)
		log.Info().Str("name", name).Msg("tunnel disabled")
	}
}
