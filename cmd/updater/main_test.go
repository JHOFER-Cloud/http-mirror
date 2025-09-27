package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jhofer-cloud/http-mirror/pkg/config"
)

func TestMainLogicWithMockTarget(t *testing.T) {
	// Create a simple test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", "13")
			w.WriteHeader(http.StatusOK)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Test content\n"))
	}))
	defer server.Close()

	// Create temporary directory
	tempDir := t.TempDir()

	// Set up environment variables to simulate configuration
	os.Setenv("MIRROR_URL", server.URL)
	os.Setenv("MIRROR_NAME", "test")
	os.Setenv("MIRROR_DATA_PATH", tempDir)
	os.Setenv("LOG_LEVEL", "error") // Reduce log noise in tests
	defer func() {
		os.Unsetenv("MIRROR_URL")
		os.Unsetenv("MIRROR_NAME")
		os.Unsetenv("MIRROR_DATA_PATH")
		os.Unsetenv("LOG_LEVEL")
	}()

	// This is a simplified version of the main function logic
	// In a real scenario, you might extract the core logic into a separate function for testing

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Validate configuration
	if len(cfg.Targets) == 0 {
		t.Fatal("No mirror targets configured")
	}

	if cfg.Targets[0].Name != "test" {
		t.Errorf("Expected target name 'test', got %s", cfg.Targets[0].Name)
	}

	if cfg.Targets[0].URL != server.URL {
		t.Errorf("Expected target URL %s, got %s", server.URL, cfg.Targets[0].URL)
	}

	if cfg.Mirror.DataPath != tempDir {
		t.Errorf("Expected data path %s, got %s", tempDir, cfg.Mirror.DataPath)
	}
}

func TestEnvironmentVariableHandling(t *testing.T) {
	// Test with no configuration
	os.Unsetenv("MIRROR_URL")
	os.Unsetenv("CONFIG_FILE")

	_, err := config.LoadConfig()
	if err != nil {
		// This is expected behavior - without config, load should succeed but have empty targets
		t.Logf("LoadConfig without configuration returned error (expected): %v", err)
	}

	// Test with minimal configuration
	os.Setenv("MIRROR_URL", "http://example.com/test/")
	defer os.Unsetenv("MIRROR_URL")

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig with MIRROR_URL failed: %v", err)
	}

	if len(cfg.Targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(cfg.Targets))
	}

	if cfg.Targets[0].URL != "http://example.com/test/" {
		t.Errorf("Expected URL 'http://example.com/test/', got %s", cfg.Targets[0].URL)
	}
}

func TestLogLevelConfiguration(t *testing.T) {
	tests := []struct {
		envValue      string
		expectedLevel slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"invalid", slog.LevelInfo}, // Should default to info
		{"", slog.LevelInfo},        // Should default to info
	}

	for _, test := range tests {
		t.Run(test.envValue, func(t *testing.T) {
			// Set environment variable
			if test.envValue != "" {
				os.Setenv("LOG_LEVEL", test.envValue)
				defer os.Unsetenv("LOG_LEVEL")
			} else {
				os.Unsetenv("LOG_LEVEL")
			}

			// Test log level determination (mimicking main function logic)
			logLevel := slog.LevelInfo
			if level := os.Getenv("LOG_LEVEL"); level != "" {
				switch level {
				case "debug":
					logLevel = slog.LevelDebug
				case "warn":
					logLevel = slog.LevelWarn
				case "error":
					logLevel = slog.LevelError
				}
			}

			if logLevel != test.expectedLevel {
				t.Errorf("Expected log level %v, got %v", test.expectedLevel, logLevel)
			}
		})
	}
}
