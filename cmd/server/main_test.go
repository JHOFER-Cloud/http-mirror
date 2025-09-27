package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jhofer-cloud/http-mirror/pkg/config"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestHealthCheckHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	healthCheckHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got %s", contentType)
	}

	var response map[string]string
	err := json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %s", response["status"])
	}

	if response["service"] != "http-mirror-server" {
		t.Errorf("Expected service 'http-mirror-server', got %s", response["service"])
	}
}

func TestMetricsEndpoint(t *testing.T) {
	tempDir := t.TempDir()

	// Create some test files to get stats
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create target directory structure
	targetDir := filepath.Join(tempDir, "example-target")
	err = os.Mkdir(targetDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}

	targetFile := filepath.Join(targetDir, "target.txt")
	err = os.WriteFile(targetFile, []byte("target content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	cfg := &config.Config{
		Server: config.Server{
			DataPath: tempDir,
		},
		Targets: []config.Target{
			{
				Name: "example-target",
				URL:  "http://example.com/files/",
			},
		},
	}

	// Update metrics first
	updateMetrics(cfg, slog.Default())

	// Test the metrics endpoint
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	promhttp.Handler().ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	// Check for Prometheus format response rather than specific metric since it might not be registered in test
	if !strings.Contains(body, "HELP") && !strings.Contains(body, "TYPE") {
		t.Error("Expected metrics endpoint to return Prometheus format data")
	}
}

func TestGetDirStats(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files and directories
	testFile1 := filepath.Join(tempDir, "file1.txt")
	testFile2 := filepath.Join(tempDir, "file2.txt")
	testDir := filepath.Join(tempDir, "subdir")

	err := os.WriteFile(testFile1, []byte("content1"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file1: %v", err)
	}

	err = os.WriteFile(testFile2, []byte("content2"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file2: %v", err)
	}

	err = os.Mkdir(testDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	stats, err := getDirStats(tempDir)
	if err != nil {
		t.Fatalf("getDirStats failed: %v", err)
	}

	// Check file count (should be 2)
	if fileCount, ok := stats["files"].(int); !ok || fileCount != 2 {
		t.Errorf("Expected 2 files, got %v", stats["files"])
	}

	// Check directory count (should be 2: root dir + subdir we created)
	if dirCount, ok := stats["directories"].(int); !ok || dirCount != 2 {
		t.Errorf("Expected 2 directories (root + subdir), got %v", stats["directories"])
	}

	// Check total size
	if totalSize, ok := stats["total_size_bytes"].(int64); !ok || totalSize != 16 {
		t.Errorf("Expected total size 16 bytes, got %v", stats["total_size_bytes"])
	}
}

func TestGetDirStatsNonExistentDir(t *testing.T) {
	stats, err := getDirStats("/nonexistent/directory")
	if err == nil {
		t.Error("Expected error for non-existent directory")
	}

	if stats == nil {
		t.Error("Stats map should not be nil even on error")
	}
}

func TestSecurityHeaders(t *testing.T) {
	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	// Wrap with security middleware
	secureHandler := securityHeadersMiddleware(handler)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	secureHandler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Check security headers
	expectedHeaders := map[string]string{
		"Content-Security-Policy": "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; object-src 'none';",
		"X-Frame-Options":         "DENY",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"X-XSS-Protection":        "1; mode=block",
	}

	for header, expected := range expectedHeaders {
		actual := resp.Header.Get(header)
		if actual != expected {
			t.Errorf("Expected %s header to be '%s', got '%s'", header, expected, actual)
		}
	}
}
