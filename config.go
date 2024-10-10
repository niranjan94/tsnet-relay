package main

import (
	"context"
	"encoding/json"
	"os"
	"sync"
)

// Config represents the application configuration
type Config struct {
	Tunnels []Tunnel `json:"tunnels"`
}

var (
	config      Config
	configMutex sync.RWMutex
)

// loadConfig reads and parses the configuration file
func loadConfig() error {
	file, err := os.Open(configPath)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	configMutex.Lock()
	defer configMutex.Unlock()
	err = decoder.Decode(&config)
	// resolve env vars on tunnel
	for _, tunnel := range config.Tunnels {
		tunnel.Source = os.ExpandEnv(tunnel.Source)
		tunnel.Destination = os.ExpandEnv(tunnel.Destination)
	}
	return err
}

// reloadConfig reloads the configuration and updates the tunnels
func reloadConfig(ctx context.Context) error {
	if err := loadConfig(); err != nil {
		return err
	}
	closeTunnels()
	return setupTunnels(ctx)
}
