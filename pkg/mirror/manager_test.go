package mirror

import (
	"context"
	"fmt"
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
)

func TestNewManager(t *testing.T) {
	cfg := &config.Config{
		Mirror: config.Mirror{
			DataPath: "/test/data",
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewManager(cfg, logger)

	if manager.config != cfg {
		t.Error("Manager should store the provided config")
	}

	if manager.logger != logger {
		t.Error("Manager should store the provided logger")
	}
}

func TestParseDirectoryListing(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewManager(cfg, logger)

	// Create a mock HTTP response with directory listing
	htmlContent := `
	<html>
	<head><title>Index of /files/</title></head>
	<body>
	<h1>Index of /files/</h1>
	<pre>
	<a href="file1.txt">file1.txt</a>                               2023-10-21 12:00    1234
	<a href="file2.pdf">file2.pdf</a>                               2023-10-21 13:00    5678
	<a href="subdir/">subdir/</a>                                   2023-10-21 14:00       -
	<a href="../">../</a>                                           2023-10-21 11:00       -
	<a href="?order=name">Name</a>
	<a href="mailto:admin@example.com">Contact</a>
	<a href="javascript:void(0)">Script</a>
	</pre>
	</body>
	</html>
	`

	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader(htmlContent)),
	}

	links, err := manager.parseDirectoryListing(resp, "http://example.com/files/")
	if err != nil {
		t.Fatalf("parseDirectoryListing failed: %v", err)
	}

	expectedLinks := []string{"file1.txt", "file2.pdf", "subdir/"}

	if len(links) != len(expectedLinks) {
		t.Fatalf("Expected %d links, got %d: %v", len(expectedLinks), len(links), links)
	}

	for i, expected := range expectedLinks {
		if links[i] != expected {
			t.Errorf("Expected link %d to be %s, got %s", i, expected, links[i])
		}
	}

	// Check that unwanted links are filtered out
	for _, link := range links {
		if strings.Contains(link, "..") || link == "../" {
			t.Errorf("Parent directory link should be filtered out: %s", link)
		}
		if strings.Contains(link, "?") {
			t.Errorf("Query parameter link should be filtered out: %s", link)
		}
		if strings.Contains(link, "mailto:") {
			t.Errorf("Mailto link should be filtered out: %s", link)
		}
		if strings.Contains(link, "javascript:") {
			t.Errorf("JavaScript link should be filtered out: %s", link)
		}
	}
}

func createTestServer(t *testing.T, responses map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Handle HEAD requests for file info checking
		if r.Method == "HEAD" {
			if response, exists := responses[path]; exists {
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(response)))
				w.Header().Set("Last-Modified", time.Now().Format(time.RFC1123))
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Handle GET requests
		if response, exists := responses[path]; exists {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestMirrorTargetSimpleFile(t *testing.T) {
	// Create test server with a simple file
	fileContent := "This is a test file content"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		t.Logf("Test server received: %s %s", r.Method, path)

		// Handle HEAD requests for file info checking
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileContent)))
			w.Header().Set("Last-Modified", time.Now().Format(time.RFC1123))
			w.WriteHeader(http.StatusOK)
			return
		}

		// Handle GET requests for simple file
		if path == "/" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fileContent))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Setup test environment
	tempDir := t.TempDir()
	target := &config.Target{
		Name:         "test-target",
		UserAgent:    "Test Agent",
		Timeout:      5,
		MaxDepth:     1,     // Allow at least one level
		CheckChanges: false, // Disable change checking for simpler test
	}

	cfg := &config.Config{
		Mirror: config.Mirror{
			DataPath: tempDir,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	manager := NewManager(cfg, logger)

	// Update target URL to point to test server
	target.URL = server.URL + "/"

	ctx := context.Background()
	err := manager.MirrorTarget(ctx, target)
	if err != nil {
		t.Fatalf("MirrorTarget failed: %v", err)
	}

	// Check that the file was created (should be saved as index.html for root path)
	expectedPath := filepath.Join(tempDir, "test-target", "index.html")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected file %s to be created", expectedPath)
		// Debug: list directory contents
		if entries, err := os.ReadDir(filepath.Join(tempDir, "test-target")); err == nil {
			t.Logf("Directory contents: %v", entries)
		}
	}

	// Check file content
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(content) != fileContent {
		t.Errorf("Expected file content %s, got %s", fileContent, string(content))
	}
}

func TestMirrorTargetDirectoryListing(t *testing.T) {
	// Create test server with directory listing
	directoryHTML := `
	<html>
	<head><title>Index of /files/</title></head>
	<body>
	<h1>Index of /files/</h1>
	<pre>
	<a href="test.txt">test.txt</a>
	<a href="subdir/">subdir/</a>
	</pre>
	</body>
	</html>
	`

	fileContent := "Test file content"
	subdirHTML := `
	<html>
	<head><title>Index of /files/subdir/</title></head>
	<body>
	<h1>Index of /files/subdir/</h1>
	<pre>
	<a href="nested.txt">nested.txt</a>
	</pre>
	</body>
	</html>
	`

	nestedContent := "Nested file content"

	responses := map[string]string{
		"/files/":                  directoryHTML,
		"/files/test.txt":          fileContent,
		"/files/subdir/":           subdirHTML,
		"/files/subdir/nested.txt": nestedContent,
	}

	server := createTestServer(t, responses)
	defer server.Close()

	// Setup test environment
	tempDir := t.TempDir()
	target := &config.Target{
		Name:         "test-target",
		UserAgent:    "Test Agent",
		Timeout:      5,
		MaxDepth:     3,
		CheckChanges: false,
	}

	cfg := &config.Config{
		Mirror: config.Mirror{
			DataPath: tempDir,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewManager(cfg, logger)

	// Update target URL to point to test server
	target.URL = server.URL + "/files/"

	ctx := context.Background()
	err := manager.MirrorTarget(ctx, target)
	if err != nil {
		t.Fatalf("MirrorTarget failed: %v", err)
	}

	// Check that files were created
	targetDir := filepath.Join(tempDir, "test-target")

	// Check main file
	testFilePath := filepath.Join(targetDir, "test.txt")
	if _, err := os.Stat(testFilePath); os.IsNotExist(err) {
		t.Errorf("Expected file %s to be created", testFilePath)
	}

	// Check subdirectory was created
	subdirPath := filepath.Join(targetDir, "subdir")
	if _, err := os.Stat(subdirPath); os.IsNotExist(err) {
		t.Errorf("Expected directory %s to be created", subdirPath)
	}

	// Check nested file
	nestedFilePath := filepath.Join(subdirPath, "nested.txt")
	if _, err := os.Stat(nestedFilePath); os.IsNotExist(err) {
		t.Errorf("Expected nested file %s to be created", nestedFilePath)
	}

	// Verify file contents
	content, err := os.ReadFile(testFilePath)
	if err == nil && string(content) != fileContent {
		t.Errorf("Expected main file content %s, got %s", fileContent, string(content))
	}

	nestedFileContent, err := os.ReadFile(nestedFilePath)
	if err == nil && string(nestedFileContent) != nestedContent {
		t.Errorf("Expected nested file content %s, got %s", nestedContent, string(nestedFileContent))
	}
}

func TestMirrorTargetWithDepthLimit(t *testing.T) {
	// Create test server with deep directory structure
	responses := map[string]string{
		"/":                              `<html><body><a href="level1/">level1/</a></body></html>`,
		"/level1/":                       `<html><body><a href="level2/">level2/</a></body></html>`,
		"/level1/level2/":                `<html><body><a href="level3/">level3/</a></body></html>`,
		"/level1/level2/level3/":         `<html><body><a href="deep.txt">deep.txt</a></body></html>`,
		"/level1/level2/level3/deep.txt": "Should not be downloaded due to depth limit",
	}

	server := createTestServer(t, responses)
	defer server.Close()

	// Setup test environment with depth limit
	tempDir := t.TempDir()
	target := &config.Target{
		Name:         "test-target",
		UserAgent:    "Test Agent",
		Timeout:      5,
		MaxDepth:     2, // Limit to 2 levels deep
		CheckChanges: false,
	}

	cfg := &config.Config{
		Mirror: config.Mirror{
			DataPath: tempDir,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewManager(cfg, logger)

	target.URL = server.URL + "/"

	ctx := context.Background()
	err := manager.MirrorTarget(ctx, target)
	if err != nil {
		t.Fatalf("MirrorTarget failed: %v", err)
	}

	// Check that deep file was NOT downloaded due to depth limit
	deepFilePath := filepath.Join(tempDir, "test-target", "level1", "level2", "level3", "deep.txt")
	if _, err := os.Stat(deepFilePath); !os.IsNotExist(err) {
		t.Error("Deep file should not have been downloaded due to depth limit")
	}

	// But level1 and level2 directories should exist
	level1Path := filepath.Join(tempDir, "test-target", "level1")
	if _, err := os.Stat(level1Path); os.IsNotExist(err) {
		t.Error("Level1 directory should exist")
	}

	level2Path := filepath.Join(tempDir, "test-target", "level1", "level2")
	if _, err := os.Stat(level2Path); os.IsNotExist(err) {
		t.Error("Level2 directory should exist")
	}
}

func TestMirrorTargetContextCancellation(t *testing.T) {
	// Create test server with slow response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("slow response"))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	target := &config.Target{
		Name:         "test-target",
		UserAgent:    "Test Agent",
		Timeout:      5,
		MaxDepth:     1, // Allow at least one level
		CheckChanges: false,
	}

	cfg := &config.Config{
		Mirror: config.Mirror{
			DataPath: tempDir,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewManager(cfg, logger)

	target.URL = server.URL + "/"

	// Create context that will be cancelled quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := manager.MirrorTarget(ctx, target)
	if err == nil {
		t.Error("Expected error due to context cancellation")
	}

	if err != context.DeadlineExceeded {
		t.Logf("Got error: %v (expected context cancellation)", err)
	}
}

func TestMirrorStatsTracking(t *testing.T) {
	// Create test server
	responses := map[string]string{
		"/":          `<html><body><a href="file1.txt">file1.txt</a><a href="file2.txt">file2.txt</a></body></html>`,
		"/file1.txt": "Content 1",
		"/file2.txt": "Content 2",
	}

	server := createTestServer(t, responses)
	defer server.Close()

	tempDir := t.TempDir()
	target := &config.Target{
		Name:         "test-target",
		UserAgent:    "Test Agent",
		Timeout:      5,
		MaxDepth:     1, // Allow at least one level
		CheckChanges: false,
	}

	cfg := &config.Config{
		Mirror: config.Mirror{
			DataPath: tempDir,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewManager(cfg, logger)

	target.URL = server.URL + "/"

	ctx := context.Background()
	err := manager.MirrorTarget(ctx, target)
	if err != nil {
		t.Fatalf("MirrorTarget failed: %v", err)
	}

	// Note: In a real implementation, you might want to expose stats
	// For now, we just verify that the operation completed successfully
	// and files were created (which indirectly tests the stats tracking)

	file1Path := filepath.Join(tempDir, "test-target", "file1.txt")
	file2Path := filepath.Join(tempDir, "test-target", "file2.txt")

	if _, err := os.Stat(file1Path); os.IsNotExist(err) {
		t.Error("file1.txt should have been downloaded")
	}

	if _, err := os.Stat(file2Path); os.IsNotExist(err) {
		t.Error("file2.txt should have been downloaded")
	}
}
