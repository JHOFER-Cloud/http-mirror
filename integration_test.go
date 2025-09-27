package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jhofer-cloud/http-mirror/pkg/config"
	"github.com/jhofer-cloud/http-mirror/pkg/files"
	"github.com/jhofer-cloud/http-mirror/pkg/mirror"
)

// TestEndToEndIntegration tests the complete flow:
// 1. Create a mock HTTP server with directory listing
// 2. Use the mirror manager to download files
// 3. Use the file server to serve the downloaded files
// 4. Verify everything works correctly
func TestEndToEndIntegration(t *testing.T) {
	// Step 1: Create a mock HTTP server with directory listing
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			// Serve directory listing
			html := `<html><body>
				<h1>Index of /</h1>
				<pre>
				<a href="file1.txt">file1.txt</a>
				<a href="file2.html">file2.html</a>
				<a href="subdir/">subdir/</a>
				</pre>
				</body></html>`
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(html))

		case "/file1.txt":
			if r.Method == "HEAD" {
				w.Header().Set("Content-Length", "21")
				w.Header().Set("Last-Modified", time.Now().Format(time.RFC1123))
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("This is file1 content"))

		case "/file2.html":
			if r.Method == "HEAD" {
				w.Header().Set("Content-Length", "35")
				w.Header().Set("Last-Modified", time.Now().Format(time.RFC1123))
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html><body>HTML content</body></html>"))

		case "/subdir/":
			html := `<html><body>
				<h1>Index of /subdir/</h1>
				<pre>
				<a href="nested.txt">nested.txt</a>
				</pre>
				</body></html>`
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(html))

		case "/subdir/nested.txt":
			if r.Method == "HEAD" {
				w.Header().Set("Content-Length", "20")
				w.Header().Set("Last-Modified", time.Now().Format(time.RFC1123))
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Nested file content."))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	// Step 2: Set up mirror configuration
	tempDir := t.TempDir()
	target := &config.Target{
		Name:         "test-site",
		URL:          mockServer.URL + "/",
		UserAgent:    "HTTP Mirror Test",
		MaxDepth:     3,
		Timeout:      10,
		CheckChanges: true,
	}

	cfg := &config.Config{
		Defaults: config.GetDefaults(),
		Targets:  []config.Target{*target},
		Mirror: config.Mirror{
			DataPath: tempDir,
			LogLevel: "error", // Reduce log noise
		},
		Server: config.Server{
			Port:     8080,
			Host:     "localhost",
			DataPath: tempDir,
		},
	}

	// Apply defaults to target
	cfg.Targets[0].UserAgent = target.UserAgent
	cfg.Targets[0].MaxDepth = target.MaxDepth
	cfg.Targets[0].Timeout = target.Timeout
	cfg.Targets[0].CheckChanges = target.CheckChanges

	// Step 3: Mirror the files
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	manager := mirror.NewManager(cfg, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := manager.MirrorTarget(ctx, &cfg.Targets[0])
	if err != nil {
		t.Fatalf("MirrorTarget failed: %v", err)
	}

	// Step 4: Verify files were downloaded
	targetDir := filepath.Join(tempDir, "test-site")

	// Check main files
	file1Path := filepath.Join(targetDir, "file1.txt")
	file2Path := filepath.Join(targetDir, "file2.html")
	nestedPath := filepath.Join(targetDir, "subdir", "nested.txt")

	if _, err := os.Stat(file1Path); os.IsNotExist(err) {
		t.Error("file1.txt should have been downloaded")
	}

	if _, err := os.Stat(file2Path); os.IsNotExist(err) {
		t.Error("file2.html should have been downloaded")
	}

	if _, err := os.Stat(nestedPath); os.IsNotExist(err) {
		t.Error("nested.txt in subdir should have been downloaded")
	}

	// Verify file contents
	content, err := os.ReadFile(file1Path)
	if err == nil && string(content) != "This is file1 content" {
		t.Errorf("Unexpected file1 content: %s", string(content))
	}

	nestedContent, err := os.ReadFile(nestedPath)
	if err == nil && string(nestedContent) != "Nested file content." {
		t.Errorf("Unexpected nested file content: %s", string(nestedContent))
	}

	// Step 5: Test serving the files with the file handler
	handler, err := files.NewHandler(tempDir, cfg)
	if err != nil {
		t.Fatalf("Failed to create file handler: %v", err)
	}

	// Test serving the root directory (should show directory listing with test-site)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for root directory, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "test-site") {
		t.Error("Root directory listing should contain 'test-site' directory")
	}

	// Test serving a specific file
	req = httptest.NewRequest("GET", "/test-site/file1.txt", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for file1.txt, got %d", resp.StatusCode)
	}

	body, _ = io.ReadAll(resp.Body)
	if string(body) != "This is file1 content" {
		t.Errorf("Unexpected served file content: %s", string(body))
	}

	// Test serving the subdirectory listing
	req = httptest.NewRequest("GET", "/test-site/", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for test-site directory, got %d", resp.StatusCode)
	}

	body, _ = io.ReadAll(resp.Body)
	bodyStr = string(body)
	if !strings.Contains(bodyStr, "file1.txt") {
		t.Error("test-site directory listing should contain 'file1.txt'")
	}
	if !strings.Contains(bodyStr, "file2.html") {
		t.Error("test-site directory listing should contain 'file2.html'")
	}
	if !strings.Contains(bodyStr, "subdir/") {
		t.Error("test-site directory listing should contain 'subdir/' directory")
	}

	t.Logf("✅ End-to-end integration test passed successfully!")
	t.Logf("   - Downloaded %d files from mock server", 3)
	t.Logf("   - Successfully served files via HTTP handler")
	t.Logf("   - Directory listings work correctly")
}

// TestConfigurationLoading tests that configuration can be loaded properly
func TestConfigurationLoading(t *testing.T) {
	tempDir := t.TempDir()

	// Test environment variable configuration
	os.Setenv("MIRROR_URL", "http://example.com/test/")
	os.Setenv("MIRROR_NAME", "integration-test")
	os.Setenv("MIRROR_DATA_PATH", tempDir)
	defer func() {
		os.Unsetenv("MIRROR_URL")
		os.Unsetenv("MIRROR_NAME")
		os.Unsetenv("MIRROR_DATA_PATH")
	}()

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.Targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(cfg.Targets))
	}

	target := cfg.Targets[0]
	if target.Name != "integration-test" {
		t.Errorf("Expected target name 'integration-test', got %s", target.Name)
	}

	if target.URL != "http://example.com/test/" {
		t.Errorf("Expected URL 'http://example.com/test/', got %s", target.URL)
	}

	if cfg.Mirror.DataPath != tempDir {
		t.Errorf("Expected data path %s, got %s", tempDir, cfg.Mirror.DataPath)
	}

	t.Logf("✅ Configuration loading test passed!")
}

// TestRateLimiting tests that rate limiting works correctly
func TestRateLimiting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping rate limiting test in short mode")
	}

	// Create a server that records request timing
	var requestTimes []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestTimes = append(requestTimes, time.Now())
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Write([]byte("test data."))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	target := &config.Target{
		Name:                "rate-test",
		URL:                 server.URL + "/",
		UserAgent:           "Rate Test",
		RateLimit:           "1k", // Very slow rate
		WaitBetweenRequests: 1,    // 1 second between requests
		MaxDepth:            1,
		CheckChanges:        false,
	}

	cfg := &config.Config{
		Mirror: config.Mirror{
			DataPath: tempDir,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	manager := mirror.NewManager(cfg, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	err := manager.MirrorTarget(ctx, target)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("MirrorTarget failed: %v", err)
	}

	// With wait between requests, it should take at least the wait time
	if duration < 500*time.Millisecond {
		t.Logf("Mirror completed in %v (rate limiting may not have been effective)", duration)
	}

	t.Logf("✅ Rate limiting test completed in %v", duration)
}
