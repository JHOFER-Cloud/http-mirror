package files

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewHandler(t *testing.T) {
	tempDir := t.TempDir()
	
	handler, err := NewHandler(tempDir, nil)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	
	if handler.rootPath != tempDir {
		t.Errorf("Expected rootPath %s, got %s", tempDir, handler.rootPath)
	}
	
	// Check that directory was created
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Error("Handler should create root directory if it doesn't exist")
	}
}

func TestServeFile(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create a test file
	testContent := "This is a test file"
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	handler, err := NewHandler(tempDir, nil)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	
	// Test serving the file
	req := httptest.NewRequest("GET", "/test.txt", nil)
	w := httptest.NewRecorder()
	
	handler.ServeHTTP(w, req)
	
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	
	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Expected Content-Type 'text/plain; charset=utf-8', got %s", contentType)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	
	if string(body) != testContent {
		t.Errorf("Expected body '%s', got '%s'", testContent, string(body))
	}
	
	// Check for Last-Modified header
	if resp.Header.Get("Last-Modified") == "" {
		t.Error("Expected Last-Modified header")
	}
}

func TestServeFileNotFound(t *testing.T) {
	tempDir := t.TempDir()
	
	handler, err := NewHandler(tempDir, nil)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	
	req := httptest.NewRequest("GET", "/nonexistent.txt", nil)
	w := httptest.NewRecorder()
	
	handler.ServeHTTP(w, req)
	
	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

func TestServeDirectoryWithIndex(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create index.html
	indexContent := "<html><body>Index page</body></html>"
	indexFile := filepath.Join(tempDir, "index.html")
	err := os.WriteFile(indexFile, []byte(indexContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create index.html: %v", err)
	}
	
	handler, err := NewHandler(tempDir, nil)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	
	handler.ServeHTTP(w, req)
	
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	
	if string(body) != indexContent {
		t.Errorf("Expected body '%s', got '%s'", indexContent, string(body))
	}
}

func TestServeDirectoryListing(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create some test files and directories
	testFiles := []string{"file1.txt", "file2.html", "image.png"}
	testDirs := []string{"subdir1", "subdir2"}
	
	for _, file := range testFiles {
		filePath := filepath.Join(tempDir, file)
		err := os.WriteFile(filePath, []byte("test content"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", file, err)
		}
	}
	
	for _, dir := range testDirs {
		dirPath := filepath.Join(tempDir, dir)
		err := os.Mkdir(dirPath, 0755)
		if err != nil {
			t.Fatalf("Failed to create test directory %s: %v", dir, err)
		}
	}
	
	handler, err := NewHandler(tempDir, nil)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	
	handler.ServeHTTP(w, req)
	
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected Content-Type to contain 'text/html', got %s", contentType)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	
	bodyStr := string(body)
	
	// Check that all files and directories are listed
	for _, file := range testFiles {
		if !strings.Contains(bodyStr, file) {
			t.Errorf("Directory listing should contain file %s", file)
		}
	}
	
	for _, dir := range testDirs {
		if !strings.Contains(bodyStr, dir+"/") {
			t.Errorf("Directory listing should contain directory %s/", dir)
		}
	}
	
	// Check for HTML structure
	if !strings.Contains(bodyStr, "<html>") || !strings.Contains(bodyStr, "</html>") {
		t.Error("Directory listing should be valid HTML")
	}
	
	if !strings.Contains(bodyStr, "Index of") {
		t.Error("Directory listing should contain title")
	}
}

func TestSecurityPathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create a file outside the temp directory
	outsideFile := filepath.Join(filepath.Dir(tempDir), "outside.txt")
	err := os.WriteFile(outsideFile, []byte("secret content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create outside file: %v", err)
	}
	defer os.Remove(outsideFile)
	
	handler, err := NewHandler(tempDir, nil)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	
	// Try to access file outside root directory
	req := httptest.NewRequest("GET", "/../outside.txt", nil)
	w := httptest.NewRecorder()
	
	handler.ServeHTTP(w, req)
	
	resp := w.Result()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("Expected status 403 for path traversal, got %d", resp.StatusCode)
	}
}

func TestGetContentType(t *testing.T) {
	tests := []struct {
		ext      string
		expected string
	}{
		{".html", "text/html; charset=utf-8"},
		{".css", "text/css; charset=utf-8"},
		{".js", "application/javascript; charset=utf-8"},
		{".json", "application/json; charset=utf-8"},
		{".pdf", "application/pdf"},
		{".jpg", "image/jpeg"},
		{".png", "image/png"},
		{".zip", "application/zip"},
		{".txt", "text/plain; charset=utf-8"},
		{".unknown", "application/octet-stream"},
		{"", "application/octet-stream"},
	}
	
	for _, test := range tests {
		t.Run(test.ext, func(t *testing.T) {
			result := getContentType(test.ext)
			if result != test.expected {
				t.Errorf("getContentType(%s) = %s, expected %s", test.ext, result, test.expected)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{2048, "2.0 KB"},
	}
	
	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			result := formatSize(test.bytes)
			if result != test.expected {
				t.Errorf("formatSize(%d) = %s, expected %s", test.bytes, result, test.expected)
			}
		})
	}
}

func TestServeWithIfModifiedSince(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create a test file
	testContent := "This is a test file"
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	handler, err := NewHandler(tempDir, nil)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	
	// First request to get the Last-Modified header
	req := httptest.NewRequest("GET", "/test.txt", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	resp := w.Result()
	lastModified := resp.Header.Get("Last-Modified")
	if lastModified == "" {
		t.Fatal("First request should return Last-Modified header")
	}
	
	// Second request with If-Modified-Since header
	req = httptest.NewRequest("GET", "/test.txt", nil)
	req.Header.Set("If-Modified-Since", lastModified)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	resp = w.Result()
	if resp.StatusCode != http.StatusNotModified {
		t.Errorf("Expected status 304, got %d", resp.StatusCode)
	}
}

func TestDirectoryListingSort(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create files and directories in specific order to test sorting
	files := []string{"zebra.txt", "apple.txt"}
	dirs := []string{"zoo", "animals"}
	
	for _, file := range files {
		filePath := filepath.Join(tempDir, file)
		err := os.WriteFile(filePath, []byte("content"), 0644)
		if err != nil {
			t.Fatalf("Failed to create file %s: %v", file, err)
		}
	}
	
	for _, dir := range dirs {
		dirPath := filepath.Join(tempDir, dir)
		err := os.Mkdir(dirPath, 0755)
		if err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}
	
	handler, err := NewHandler(tempDir, nil)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	
	bodyStr := string(body)
	
	// Check that directories come before files (directories should be listed first)
	animalsPos := strings.Index(bodyStr, "animals/")
	zooPos := strings.Index(bodyStr, "zoo/")
	applePos := strings.Index(bodyStr, "apple.txt")
	zebraPos := strings.Index(bodyStr, "zebra.txt")
	
	if animalsPos == -1 || zooPos == -1 || applePos == -1 || zebraPos == -1 {
		t.Fatal("Not all files/directories found in listing")
	}
	
	// Directories should come before files
	if animalsPos > applePos || zooPos > applePos {
		t.Error("Directories should be listed before files")
	}
	
	// Within each category, items should be sorted alphabetically
	if animalsPos > zooPos {
		t.Error("Directories should be sorted alphabetically (animals before zoo)")
	}
	
	if applePos > zebraPos {
		t.Error("Files should be sorted alphabetically (apple before zebra)")
	}
}