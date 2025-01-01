package tsnet_relay

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"sync"

	"github.com/rs/zerolog/log"
)

// Config represents the application configuration
type Config struct {
	Tunnels []Tunnel `json:"tunnels"`
}

var (
	config      Config
	configMutex sync.RWMutex
)

// LoadConfig reads and parses the configuration file
func LoadConfig(configPath string) error {
	rawConfigFromEnv := os.Getenv("TSNET_RELAY_CONFIG")
	if rawConfigFromEnv != "" {
		log.Printf("loading config from environment variable `TSNET_RELAY_CONFIG`")
		configMutex.Lock()
		defer configMutex.Unlock()
		err := json.Unmarshal([]byte(rawConfigFromEnv), &config)
		if err != nil {
			return err
		}
	} else {
		log.Printf("loading config from file `%s`", configPath)
		file, err := os.Open(configPath)
		if err != nil {
			return err
		}
		defer file.Close()
		decoder := json.NewDecoder(file)
		configMutex.Lock()
		defer configMutex.Unlock()
		if err := decoder.Decode(&config); err != nil {
			return err
		}
	}

	// resolve env vars on tunnel
	for _, tunnel := range config.Tunnels {
		tunnel.Source = os.ExpandEnv(tunnel.Source)
		tunnel.Destination = os.ExpandEnv(tunnel.Destination)
	}
	return nil
}

// ReloadConfig reloads the configuration and updates the tunnels
func ReloadConfig(ctx context.Context, configPath string, srv *Server) error {
	log.Info().Msg("reloading configuration")

	oldTunnels := make([]Tunnel, len(config.Tunnels))
	copy(oldTunnels, config.Tunnels)

	if err := LoadConfig(configPath); err != nil {
		return err
	}

	// Compare old and new configurations
	tunnelsToStop, tunnelsToStart := compareTunnelConfigs(oldTunnels, config.Tunnels)

	// Stop changed tunnels
	for _, tunnel := range tunnelsToStop {
		log.Info().Str("name", tunnel.Name).Msg("stopping tunnel")
		srv.StopTunnel(tunnel.Name)
	}

	// Start new or changed tunnels
	for _, tunnel := range tunnelsToStart {
		log.Info().Str("name", tunnel.Name).Msg("starting tunnel")
		go srv.manageTunnel(ctx, ctx, tunnel)
	}
	return nil
}

// compareTunnelConfigs compares old and new tunnel configurations
func compareTunnelConfigs(oldTunnels, newTunnels []Tunnel) ([]Tunnel, []Tunnel) {
	var tunnelsToStop, tunnelsToStart []Tunnel
	oldMap := make(map[string]Tunnel)
	newMap := make(map[string]Tunnel)

	for _, t := range oldTunnels {
		oldMap[t.Name] = t
	}
	for _, t := range newTunnels {
		newMap[t.Name] = t
	}

	log.Info().Interface("old", oldMap).Interface("new", newMap).Msg("comparing tunnel configurations")
	for name, oldTunnel := range oldMap {
		newTunnel, exists := newMap[name]
		if !exists || !reflect.DeepEqual(oldTunnel, newTunnel) {
			tunnelsToStop = append(tunnelsToStop, oldTunnel)
		}
	}

	for name, newTunnel := range newMap {
		oldTunnel, exists := oldMap[name]
		if !exists || !reflect.DeepEqual(oldTunnel, newTunnel) {
			tunnelsToStart = append(tunnelsToStart, newTunnel)
		}
	}

	log.Info().Interface("tunnelsToStop", tunnelsToStop).Interface("tunnelsToStart", tunnelsToStart).Msg("tunnels to stop and start")
	return tunnelsToStop, tunnelsToStart
}
