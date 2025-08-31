package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jhofer-cloud/http-mirror/pkg/config"
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

func TestStatusHandler(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create some test files to get stats
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	cfg := &config.Config{
		Server: config.Server{
			DataPath: tempDir,
		},
	}
	
	handler := statusHandler(cfg)
	
	req := httptest.NewRequest("GET", "/status", nil)
	w := httptest.NewRecorder()
	
	handler(w, req)
	
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got %s", contentType)
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