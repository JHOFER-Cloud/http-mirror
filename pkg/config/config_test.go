package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetDefaults(t *testing.T) {
	defaults := GetDefaults()
	
	if defaults.UserAgent == "" {
		t.Error("UserAgent should not be empty")
	}
	
	if defaults.RateLimit != "500k" {
		t.Errorf("Expected RateLimit to be '500k', got %s", defaults.RateLimit)
	}
	
	if defaults.Retries != 3 {
		t.Errorf("Expected Retries to be 3, got %d", defaults.Retries)
	}
	
	if defaults.MaxDepth != 5 {
		t.Errorf("Expected MaxDepth to be 5, got %d", defaults.MaxDepth)
	}
	
	if !defaults.Timestamping {
		t.Error("Timestamping should be true by default")
	}
	
	if !defaults.CheckChanges {
		t.Error("CheckChanges should be true by default")
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("MIRROR_URL", "http://example.com/files/")
	os.Setenv("MIRROR_NAME", "test-mirror")
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("MIRROR_URL")
		os.Unsetenv("MIRROR_NAME")
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("LOG_LEVEL")
	}()
	
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	
	if len(config.Targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(config.Targets))
	}
	
	target := config.Targets[0]
	if target.Name != "test-mirror" {
		t.Errorf("Expected target name 'test-mirror', got %s", target.Name)
	}
	
	if target.URL != "http://example.com/files/" {
		t.Errorf("Expected target URL 'http://example.com/files/', got %s", target.URL)
	}
	
	if config.Server.Port != 9090 {
		t.Errorf("Expected server port 9090, got %d", config.Server.Port)
	}
	
	if config.Mirror.LogLevel != "debug" {
		t.Errorf("Expected log level 'debug', got %s", config.Mirror.LogLevel)
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.json")
	
	configData := Config{
		Defaults: Defaults{
			UserAgent:   "Test User Agent",
			RateLimit:   "100k",
			Retries:     5,
			MaxDepth:    10,
			Timeout:     60,
			Timestamping: false,
			CheckChanges: false,
		},
		Targets: []Target{
			{
				Name:     "test1",
				URL:      "http://test1.com/",
				Retries:  10, // Override default
			},
			{
				Name:    "test2",
				URL:     "http://test2.com/",
				Timeout: 120, // Override default
			},
		},
		Mirror: Mirror{
			DataPath: "/custom/data",
			LogLevel: "warn",
		},
		Server: Server{
			Port: 8888,
			Host: "127.0.0.1",
		},
	}
	
	data, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}
	
	err = os.WriteFile(configFile, data, 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	
	// Set environment variable to use the config file
	os.Setenv("CONFIG_FILE", configFile)
	defer os.Unsetenv("CONFIG_FILE")
	
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	
	// Check defaults
	if config.Defaults.UserAgent != "Test User Agent" {
		t.Errorf("Expected UserAgent 'Test User Agent', got %s", config.Defaults.UserAgent)
	}
	
	// Check targets
	if len(config.Targets) != 2 {
		t.Fatalf("Expected 2 targets, got %d", len(config.Targets))
	}
	
	// Check target 1 with overridden retries
	target1 := config.Targets[0]
	if target1.Name != "test1" {
		t.Errorf("Expected target1 name 'test1', got %s", target1.Name)
	}
	if target1.Retries != 10 {
		t.Errorf("Expected target1 retries 10, got %d", target1.Retries)
	}
	if target1.MaxDepth != 10 { // Should inherit from defaults
		t.Errorf("Expected target1 maxDepth 10 (from defaults), got %d", target1.MaxDepth)
	}
	
	// Check target 2 with overridden timeout
	target2 := config.Targets[1]
	if target2.Name != "test2" {
		t.Errorf("Expected target2 name 'test2', got %s", target2.Name)
	}
	if target2.Timeout != 120 {
		t.Errorf("Expected target2 timeout 120, got %d", target2.Timeout)
	}
	if target2.Retries != 5 { // Should inherit from defaults
		t.Errorf("Expected target2 retries 5 (from defaults), got %d", target2.Retries)
	}
	
	// Check other config sections
	if config.Server.Port != 8888 {
		t.Errorf("Expected server port 8888, got %d", config.Server.Port)
	}
	
	if config.Mirror.DataPath != "/custom/data" {
		t.Errorf("Expected data path '/custom/data', got %s", config.Mirror.DataPath)
	}
}

func TestApplyDefaults(t *testing.T) {
	defaults := Defaults{
		UserAgent:   "Default Agent",
		RateLimit:   "500k",
		Retries:     3,
		MaxDepth:    5,
		Timeout:     30,
		Timestamping: true,
		CheckChanges: true,
	}
	
	target := Target{
		Name:    "test",
		URL:     "http://test.com/",
		Retries: 10, // Should override default
		// Other fields should use defaults
	}
	
	applyDefaults(&target, defaults)
	
	// Check that override is preserved
	if target.Retries != 10 {
		t.Errorf("Expected retries 10 (override), got %d", target.Retries)
	}
	
	// Check that defaults are applied
	if target.UserAgent != "Default Agent" {
		t.Errorf("Expected UserAgent 'Default Agent' (from defaults), got %s", target.UserAgent)
	}
	
	if target.RateLimit != "500k" {
		t.Errorf("Expected RateLimit '500k' (from defaults), got %s", target.RateLimit)
	}
	
	if target.MaxDepth != 5 {
		t.Errorf("Expected MaxDepth 5 (from defaults), got %d", target.MaxDepth)
	}
	
	if !target.Timestamping {
		t.Error("Expected Timestamping true (from defaults)")
	}
}

func TestTargetGetTimeout(t *testing.T) {
	target := Target{
		Timeout: 45,
	}
	
	timeout := target.GetTimeout()
	expected := 45 * time.Second
	
	if timeout != expected {
		t.Errorf("Expected timeout %v, got %v", expected, timeout)
	}
}

func TestTargetGetWaitDuration(t *testing.T) {
	target := Target{
		WaitBetweenRequests: 3,
	}
	
	wait := target.GetWaitDuration()
	expected := 3 * time.Second
	
	if wait != expected {
		t.Errorf("Expected wait duration %v, got %v", expected, wait)
	}
}

func TestGetEnvWithDefault(t *testing.T) {
	// Test with existing environment variable
	os.Setenv("TEST_VAR", "test_value")
	defer os.Unsetenv("TEST_VAR")
	
	result := getEnv("TEST_VAR", "default_value")
	if result != "test_value" {
		t.Errorf("Expected 'test_value', got %s", result)
	}
	
	// Test with non-existing environment variable
	result = getEnv("NON_EXISTING_VAR", "default_value")
	if result != "default_value" {
		t.Errorf("Expected 'default_value', got %s", result)
	}
}

func TestGetEnvIntWithDefault(t *testing.T) {
	// Test with valid integer environment variable
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")
	
	result := getEnvInt("TEST_INT", 10)
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}
	
	// Test with invalid integer environment variable
	os.Setenv("TEST_INVALID_INT", "not_a_number")
	defer os.Unsetenv("TEST_INVALID_INT")
	
	result = getEnvInt("TEST_INVALID_INT", 10)
	if result != 10 {
		t.Errorf("Expected 10 (default), got %d", result)
	}
	
	// Test with non-existing environment variable
	result = getEnvInt("NON_EXISTING_INT", 20)
	if result != 20 {
		t.Errorf("Expected 20 (default), got %d", result)
	}
}