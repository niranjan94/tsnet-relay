package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Tunnel represents a single tunnel configuration
type Tunnel struct {
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

const (
	checkInterval = 30 * time.Second
)

var (
	activeTunnels      = make(map[string]net.Listener)
	activeTunnelsMutex sync.Mutex
)

// setupTunnels initializes all enabled tunnels
func setupTunnels(ctx context.Context) error {
	configMutex.RLock()
	defer configMutex.RUnlock()
	for _, tunnel := range config.Tunnels {
		if tunnel.Enabled {
			log.Info().Str("name", tunnel.Name).Str("source", tunnel.Source).Str("destination", tunnel.Destination).Msg("enabling tunnel")
			go manageTunnel(ctx, tunnel)
		}
	}
	return nil
}

// manageTunnel handles the lifecycle of a single tunnel
func manageTunnel(ctx context.Context, tunnel Tunnel) {
	for {
		err := setupTunnel(ctx, tunnel)
		if err == nil {
			// Tunnel is set up successfully, our job here is done
			return
		}

		log.Error().Err(err).Str("tunnel", tunnel.Name).Msg("Failed to set up tunnel, will retry")

		select {
		case <-ctx.Done():
			return
		case <-time.After(checkInterval):
			// Continue to next iteration and try again
		}
	}
}

// setupTunnel sets up a single tunnel
func setupTunnel(ctx context.Context, tunnel Tunnel) error {
	sourceProto, sourceAddr, err := parseAddress(tunnel.Source)
	if err != nil {
		return err
	}

	destProto, destAddr, err := parseAddress(tunnel.Destination)
	if err != nil {
		return err
	}

	// Check if the destination is a Tailscale service
	if strings.HasPrefix(destProto, "tcp+tailnet") {
		// Try to connect to the destination
		conn, err := srv.Dial(ctx, "tcp", destAddr)
		if err != nil {
			return fmt.Errorf("destination %s is not accessible: %v", tunnel.Destination, err)
		}
		conn.Close() // Close the connection immediately after successful dial
		log.Info().Str("destination", tunnel.Destination).Msg("Tailscale destination is accessible")
	}

	var listener net.Listener
	if strings.HasPrefix(sourceProto, "tcp+tailnet") {
		fmt.Println("srv.Listen", sourceAddr, sourceProto)
		listener, err = srv.Listen("tcp", sourceAddr)
	} else {
		listener, err = net.Listen(sourceProto, sourceAddr)
	}
	if err != nil {
		return err
	}

	activeTunnelsMutex.Lock()
	activeTunnels[tunnel.Name] = listener
	activeTunnelsMutex.Unlock()

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
					log.Error().Err(err).Str("tunnel", tunnel.Name).Msg("Failed to accept connection")
					continue
				}
			}

			go handleConnection(ctx, conn, destProto, destAddr)
		}
	}()

	return nil
}

// handleConnection manages a single connection through the tunnel
func handleConnection(ctx context.Context, clientConn net.Conn, destProto, destAddr string) {
	defer clientConn.Close()

	log.Info().Str("client", clientConn.RemoteAddr().String()).Str("destination", destAddr).Msg("New connection")

	var remoteConn net.Conn
	var err error

	if strings.HasPrefix(destProto, "tcp+tailnet") {
		remoteConn, err = srv.Dial(ctx, "tcp", destAddr)
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

	log.Info().Str("client", clientConn.RemoteAddr().String()).Msg("Connection closed")
}

// parseAddress splits an address into protocol and address parts
func parseAddress(addr string) (string, string, error) {
	parts := strings.SplitN(addr, "://", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid address format: %s", addr)
	}
	return parts[0], parts[1], nil
}

// closeTunnels closes all active tunnels
func closeTunnels() {
	activeTunnelsMutex.Lock()
	defer activeTunnelsMutex.Unlock()

	for name, listener := range activeTunnels {
		listener.Close()
		delete(activeTunnels, name)
		log.Info().Str("name", name).Msg("Tunnel disabled")
	}
}
