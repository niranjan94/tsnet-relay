package tsnet_relay

import (
	"fmt"
	"strings"
)

// Tunnel represents a single tunnel configuration
type Tunnel struct {
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Source      string `json:"source"`
	Primary     bool   `json:"primary"`
	Destination string `json:"destination"`
}

// parseAddress splits an address into protocol and address parts
func parseAddress(addr string) (string, string, error) {
	parts := strings.SplitN(addr, "://", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid address format: %s", addr)
	}
	return parts[0], parts[1], nil
}
