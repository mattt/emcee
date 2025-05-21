package config

import (
	"bytes"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	
	// Verify all operations are enabled by default
	if cfg.DisabledOperations.GET {
		t.Error("GET should be enabled by default")
	}
	if cfg.DisabledOperations.POST {
		t.Error("POST should be enabled by default")
	}
	if cfg.DisabledOperations.PUT {
		t.Error("PUT should be enabled by default")
	}
	if cfg.DisabledOperations.DELETE {
		t.Error("DELETE should be enabled by default")
	}
	if cfg.DisabledOperations.PATCH {
		t.Error("PATCH should be enabled by default")
	}
	if cfg.DisabledOperations.HEAD {
		t.Error("HEAD should be enabled by default")
	}
	if cfg.DisabledOperations.OPTIONS {
		t.Error("OPTIONS should be enabled by default")
	}
	
	// Verify empty disabled endpoints and paths
	if len(cfg.DisabledEndpoints) != 0 {
		t.Error("DisabledEndpoints should be empty by default")
	}
	if len(cfg.DisabledPaths) != 0 {
		t.Error("DisabledPaths should be empty by default")
	}
}

func TestLoad(t *testing.T) {
	jsonConfig := `{
		"disabledOperations": {
			"get": false,
			"post": false,
			"put": false,
			"delete": true,
			"patch": false,
			"head": true,
			"options": false
		},
		"disabledEndpoints": [
			"createUser",
			"deleteItem"
		],
		"disabledPaths": [
			"/admin/.*"
		]
	}`
	
	cfg, err := Load(bytes.NewBufferString(jsonConfig))
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	// Verify disabled operations
	if cfg.DisabledOperations.GET {
		t.Error("GET should be enabled")
	}
	if cfg.DisabledOperations.DELETE != true {
		t.Error("DELETE should be disabled")
	}
	if cfg.DisabledOperations.HEAD != true {
		t.Error("HEAD should be disabled")
	}
	
	// Verify disabled endpoints
	if len(cfg.DisabledEndpoints) != 2 {
		t.Errorf("Expected 2 disabled endpoints, got %d", len(cfg.DisabledEndpoints))
	}
	if cfg.DisabledEndpoints[0] != "createUser" {
		t.Errorf("Expected first disabled endpoint to be 'createUser', got '%s'", cfg.DisabledEndpoints[0])
	}
	if cfg.DisabledEndpoints[1] != "deleteItem" {
		t.Errorf("Expected second disabled endpoint to be 'deleteItem', got '%s'", cfg.DisabledEndpoints[1])
	}
	
	// Verify disabled paths
	if len(cfg.DisabledPaths) != 1 {
		t.Errorf("Expected 1 disabled path, got %d", len(cfg.DisabledPaths))
	}
	if cfg.DisabledPaths[0] != "/admin/.*" {
		t.Errorf("Expected disabled path to be '/admin/.*', got '%s'", cfg.DisabledPaths[0])
	}
}

func TestIsOperationDisabled(t *testing.T) {
	cfg := &EmceeConfig{
		DisabledOperations: Operations{
			GET:     false,
			POST:    false,
			PUT:     false,
			DELETE:  true,
			PATCH:   false,
			HEAD:    true,
			OPTIONS: false,
		},
	}
	
	// Test cases
	testCases := []struct {
		method   string
		expected bool
	}{
		{"GET", false},
		{"get", false},
		{"POST", false},
		{"PUT", false},
		{"DELETE", true},
		{"delete", true},
		{"PATCH", false},
		{"HEAD", true},
		{"OPTIONS", false},
		{"UNKNOWN", false}, // Unknown methods should not be disabled
	}
	
	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			result := cfg.IsOperationDisabled(tc.method)
			if result != tc.expected {
				t.Errorf("IsOperationDisabled(%s) = %v, expected %v", tc.method, result, tc.expected)
			}
		})
	}
}

func TestIsEndpointDisabled(t *testing.T) {
	cfg := &EmceeConfig{
		DisabledEndpoints: []string{"createUser", "deleteItem"},
	}
	
	// Test cases
	testCases := []struct {
		operationID string
		expected    bool
	}{
		{"createUser", true},
		{"updateUser", false},
		{"deleteItem", true},
		{"getItems", false},
		{"", false}, // Empty operation ID should not be disabled
	}
	
	for _, tc := range testCases {
		t.Run(tc.operationID, func(t *testing.T) {
			result := cfg.IsEndpointDisabled(tc.operationID)
			if result != tc.expected {
				t.Errorf("IsEndpointDisabled(%s) = %v, expected %v", tc.operationID, result, tc.expected)
			}
		})
	}
}
