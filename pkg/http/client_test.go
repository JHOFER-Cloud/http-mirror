package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jhofer-cloud/http-mirror/pkg/config"
)

func TestNewClient(t *testing.T) {
	target := &config.Target{
		UserAgent: "Test Agent",
		RateLimit: "100k",
		Timeout:   30,
	}
	
	client := NewClient(target)
	
	if client.config.UserAgent != "Test Agent" {
		t.Errorf("Expected UserAgent 'Test Agent', got %s", client.config.UserAgent)
	}
	
	if client.GetUserAgent() != "Test Agent" {
		t.Errorf("Expected GetUserAgent() 'Test Agent', got %s", client.GetUserAgent())
	}
	
	if client.GetConfig() != target {
		t.Error("GetConfig() should return the original target")
	}
	
	if client.client.Timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", client.client.Timeout)
	}
}

func TestParseRateLimit(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"100", 100},
		{"100k", 100 * 1024},
		{"1m", 1 * 1024 * 1024},
		{"2g", 2 * 1024 * 1024 * 1024},
		{"invalid", 0},
		{"", 0},
		{"100x", 0}, // Invalid suffix
	}
	
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := parseRateLimit(test.input)
			if result != test.expected {
				t.Errorf("parseRateLimit(%s) = %d, expected %d", test.input, result, test.expected)
			}
		})
	}
}

func TestCheckFileInfo(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" {
			t.Errorf("Expected HEAD request, got %s", r.Method)
		}
		
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2023 07:28:00 GMT")
		w.Header().Set("Content-Length", "12345")
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	
	target := &config.Target{
		UserAgent: "Test Agent",
		Timeout:   5,
	}
	
	client := NewClient(target)
	ctx := context.Background()
	
	info, err := client.CheckFileInfo(ctx, server.URL)
	if err != nil {
		t.Fatalf("CheckFileInfo failed: %v", err)
	}
	
	if info.URL != server.URL {
		t.Errorf("Expected URL %s, got %s", server.URL, info.URL)
	}
	
	if info.Size != 12345 {
		t.Errorf("Expected Size 12345, got %d", info.Size)
	}
	
	if info.ETag != `"abc123"` {
		t.Errorf("Expected ETag '\"abc123\"', got %s", info.ETag)
	}
	
	if info.ContentType != "text/html" {
		t.Errorf("Expected ContentType 'text/html', got %s", info.ContentType)
	}
	
	expectedTime, _ := time.Parse(time.RFC1123, "Wed, 21 Oct 2023 07:28:00 GMT")
	if !info.LastModified.Equal(expectedTime) {
		t.Errorf("Expected LastModified %v, got %v", expectedTime, info.LastModified)
	}
}

func TestCheckFileInfoError(t *testing.T) {
	target := &config.Target{
		UserAgent: "Test Agent",
		Timeout:   1, // Short timeout
	}
	
	client := NewClient(target)
	ctx := context.Background()
	
	// Test with invalid URL
	_, err := client.CheckFileInfo(ctx, "invalid-url")
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
	
	// Test with server that returns error status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	
	_, err = client.CheckFileInfo(ctx, server.URL)
	if err == nil {
		t.Error("Expected error for 404 status")
	}
}

func TestNeedsUpdate(t *testing.T) {
	tempDir := t.TempDir()
	target := &config.Target{
		UserAgent: "Test Agent",
	}
	client := NewClient(target)
	
	// Test with non-existing file
	localPath := filepath.Join(tempDir, "nonexistent.txt")
	remoteInfo := &FileInfo{
		LastModified: time.Now(),
		Size:         100,
	}
	
	needsUpdate, err := client.NeedsUpdate(localPath, remoteInfo)
	if err != nil {
		t.Fatalf("NeedsUpdate failed: %v", err)
	}
	if !needsUpdate {
		t.Error("Expected needsUpdate=true for non-existing file")
	}
	
	// Create a local file
	localPath = filepath.Join(tempDir, "existing.txt")
	content := "test content"
	err = os.WriteFile(localPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	// Test with older remote file
	stat, _ := os.Stat(localPath)
	remoteInfo = &FileInfo{
		LastModified: stat.ModTime().Add(-time.Hour), // 1 hour older
		Size:         int64(len(content)),
	}
	
	needsUpdate, err = client.NeedsUpdate(localPath, remoteInfo)
	if err != nil {
		t.Fatalf("NeedsUpdate failed: %v", err)
	}
	if needsUpdate {
		t.Error("Expected needsUpdate=false for older remote file")
	}
	
	// Test with newer remote file
	remoteInfo = &FileInfo{
		LastModified: stat.ModTime().Add(time.Hour), // 1 hour newer
		Size:         int64(len(content)),
	}
	
	needsUpdate, err = client.NeedsUpdate(localPath, remoteInfo)
	if err != nil {
		t.Fatalf("NeedsUpdate failed: %v", err)
	}
	if !needsUpdate {
		t.Error("Expected needsUpdate=true for newer remote file")
	}
	
	// Test with different size
	remoteInfo = &FileInfo{
		LastModified: stat.ModTime(),
		Size:         int64(len(content)) + 100, // Different size
	}
	
	needsUpdate, err = client.NeedsUpdate(localPath, remoteInfo)
	if err != nil {
		t.Fatalf("NeedsUpdate failed: %v", err)
	}
	if !needsUpdate {
		t.Error("Expected needsUpdate=true for different file size")
	}
}

func TestDownloadFile(t *testing.T) {
	testContent := "This is test content for download"
	
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			// For CheckFileInfo
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testContent)))
			w.Header().Set("Last-Modified", time.Now().Format(time.RFC1123))
			w.WriteHeader(http.StatusOK)
			return
		}
		
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		
		userAgent := r.Header.Get("User-Agent")
		if userAgent != "Test Agent" {
			t.Errorf("Expected User-Agent 'Test Agent', got %s", userAgent)
		}
		
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2023 07:28:00 GMT")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testContent))
	}))
	defer server.Close()
	
	tempDir := t.TempDir()
	target := &config.Target{
		UserAgent:    "Test Agent",
		CheckChanges: true,
	}
	
	client := NewClient(target)
	ctx := context.Background()
	localPath := filepath.Join(tempDir, "downloaded.txt")
	
	err := client.DownloadFile(ctx, server.URL, localPath)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}
	
	// Check file was created
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		t.Fatal("Downloaded file does not exist")
	}
	
	// Check file content
	content, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}
	
	if string(content) != testContent {
		t.Errorf("Expected content %s, got %s", testContent, string(content))
	}
}

func TestDownloadFileSkipUnchanged(t *testing.T) {
	testContent := "This is test content"
	
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			// Return same modification time and size as existing file
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testContent)))
			w.Header().Set("Last-Modified", "Wed, 21 Oct 2023 07:28:00 GMT")
			w.WriteHeader(http.StatusOK)
			return
		}
		
		// If we reach here, it means the file was downloaded (which shouldn't happen)
		t.Error("File should not have been downloaded - it's unchanged")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	
	tempDir := t.TempDir()
	target := &config.Target{
		UserAgent:    "Test Agent",
		CheckChanges: true,
	}
	
	// Create existing file with specific mod time
	localPath := filepath.Join(tempDir, "existing.txt")
	err := os.WriteFile(localPath, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}
	
	// Set the mod time to match what the server reports
	modTime, _ := time.Parse(time.RFC1123, "Wed, 21 Oct 2023 07:28:00 GMT")
	err = os.Chtimes(localPath, modTime, modTime)
	if err != nil {
		t.Fatalf("Failed to set file mod time: %v", err)
	}
	
	client := NewClient(target)
	ctx := context.Background()
	
	// This should not trigger a download
	err = client.DownloadFile(ctx, server.URL, localPath)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}
}

func TestRateLimitedReader(t *testing.T) {
	// This test is more complex to test properly, so we'll just test basic functionality
	content := "test content for rate limiting"
	reader := io.NopCloser(strings.NewReader(content))
	
	target := &config.Target{
		RateLimit: "1k", // Very low rate limit
	}
	client := NewClient(target)
	
	rateLimited := &rateLimitedReader{
		reader:  reader,
		limiter: client.limiter,
		ctx:     context.Background(),
	}
	
	buffer := make([]byte, 5)
	n, err := rateLimited.Read(buffer)
	if err != nil {
		t.Fatalf("rateLimitedReader.Read failed: %v", err)
	}
	
	if n != 5 {
		t.Errorf("Expected to read 5 bytes, got %d", n)
	}
	
	if string(buffer) != "test " {
		t.Errorf("Expected to read 'test ', got '%s'", string(buffer))
	}
	
	// Test Close
	err = rateLimited.Close()
	if err != nil {
		t.Errorf("rateLimitedReader.Close failed: %v", err)
	}
}