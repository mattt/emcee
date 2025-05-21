package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// EmceeConfig represents the configuration for the emcee service
type EmceeConfig struct {
	// DisabledOperations specifies which HTTP operations are disabled
	DisabledOperations Operations `json:"disabledOperations"`
	
	// DisabledEndpoints specifies which specific endpoints are disabled
	DisabledEndpoints []string `json:"disabledEndpoints"`
	
	// DisabledPaths specifies which paths (as regex patterns) are disabled
	DisabledPaths []string `json:"disabledPaths"`
}

// Operations represents which HTTP operations are enabled/disabled
type Operations struct {
	GET     bool `json:"get"`
	POST    bool `json:"post"`
	PUT     bool `json:"put"`
	DELETE  bool `json:"delete"`
	PATCH   bool `json:"patch"`
	HEAD    bool `json:"head"`
	OPTIONS bool `json:"options"`
}

// DefaultConfig returns a default configuration with all operations enabled
func DefaultConfig() *EmceeConfig {
	return &EmceeConfig{
		DisabledOperations: Operations{
			GET:     false,
			POST:    false,
			PUT:     false,
			DELETE:  false,
			PATCH:   false,
			HEAD:    false,
			OPTIONS: false,
		},
		DisabledEndpoints: []string{},
		DisabledPaths:     []string{},
	}
}

// LoadFile loads configuration from a file
func LoadFile(path string) (*EmceeConfig, error) {
	if path == "" {
		return DefaultConfig(), nil
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("error opening config file: %w", err)
	}
	defer f.Close()

	return Load(f)
}

// Load loads configuration from an io.Reader
func Load(r io.Reader) (*EmceeConfig, error) {
	config := DefaultConfig()
	
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("error reading config data: %w", err)
	}
	
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("error parsing config JSON: %w", err)
	}
	
	return config, nil
}

// IsOperationDisabled checks if a specific HTTP operation is disabled
func (c *EmceeConfig) IsOperationDisabled(method string) bool {
	method = strings.ToUpper(method)
	
	switch method {
	case "GET":
		return c.DisabledOperations.GET
	case "POST":
		return c.DisabledOperations.POST
	case "PUT":
		return c.DisabledOperations.PUT
	case "DELETE":
		return c.DisabledOperations.DELETE
	case "PATCH":
		return c.DisabledOperations.PATCH
	case "HEAD":
		return c.DisabledOperations.HEAD
	case "OPTIONS":
		return c.DisabledOperations.OPTIONS
	default:
		return false
	}
}

// IsEndpointDisabled checks if a specific operation ID is in the disabled list
func (c *EmceeConfig) IsEndpointDisabled(operationID string) bool {
	for _, disabled := range c.DisabledEndpoints {
		if disabled == operationID {
			return true
		}
	}
	return false
}

// Save writes the configuration to a file
func (c *EmceeConfig) Save(path string) error {
	// Create parent directories if they don't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}
	
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling config: %w", err)
	}
	
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	
	return nil
}
