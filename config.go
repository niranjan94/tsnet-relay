package main

import (
	"context"
	"encoding/json"
	"log"
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

// reloadConfig reloads the configuration and updates the tunnels
func reloadConfig(ctx context.Context) error {
	if err := loadConfig(); err != nil {
		return err
	}
	closeTunnels()
	return setupTunnels(ctx)
}
