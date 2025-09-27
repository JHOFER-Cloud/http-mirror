package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jhofer-cloud/http-mirror/pkg/config"
	"golang.org/x/time/rate"
)

// Client wraps http.Client with additional functionality
type Client struct {
	client  *http.Client
	limiter *rate.Limiter
	config  *config.Target
}

// NewClient creates a new HTTP client with rate limiting
func NewClient(target *config.Target) *Client {
	client := &http.Client{
		Timeout: target.GetTimeout(),
	}

	// Parse rate limit (e.g., "500k" -> 500KB/s)
	var limiter *rate.Limiter
	if target.RateLimit != "" {
		if bytesPerSecond := parseRateLimit(target.RateLimit); bytesPerSecond > 0 {
			limiter = rate.NewLimiter(rate.Limit(bytesPerSecond), int(bytesPerSecond))
		}
	}

	return &Client{
		client:  client,
		limiter: limiter,
		config:  target,
	}
}

// GetUserAgent returns the user agent for this client
func (c *Client) GetUserAgent() string {
	return c.config.UserAgent
}

// GetConfig returns the target configuration
func (c *Client) GetConfig() *config.Target {
	return c.config
}

// DoRequest executes an HTTP request
func (c *Client) DoRequest(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

// FileInfo represents remote file information
type FileInfo struct {
	URL          string
	LastModified time.Time
	Size         int64
	ETag         string
	ContentType  string
}

// CheckFileInfo performs a HEAD request to get file information
func (c *Client) CheckFileInfo(ctx context.Context, url string) (*FileInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HEAD request: %w", err)
	}

	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HEAD request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HEAD request returned status %d", resp.StatusCode)
	}

	info := &FileInfo{
		URL:         url,
		ContentType: resp.Header.Get("Content-Type"),
		ETag:        resp.Header.Get("ETag"),
	}

	// Parse Content-Length
	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			info.Size = size
		}
	}

	// Parse Last-Modified
	if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
		if t, err := time.Parse(time.RFC1123, lastModified); err == nil {
			info.LastModified = t
		}
	}

	return info, nil
}

// NeedsUpdate checks if a local file needs to be updated based on remote file info
func (c *Client) NeedsUpdate(localPath string, remoteInfo *FileInfo) (bool, error) {
	// If file doesn't exist locally, we need to download it
	stat, err := os.Stat(localPath)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to stat local file: %w", err)
	}

	// Check if remote file is newer
	if !remoteInfo.LastModified.IsZero() && stat.ModTime().Before(remoteInfo.LastModified) {
		return true, nil
	}

	// Check if size differs (simple change detection)
	if remoteInfo.Size > 0 && stat.Size() != remoteInfo.Size {
		return true, nil
	}

	// File appears to be up to date
	return false, nil
}

// DownloadFile downloads a file with rate limiting and progress tracking
func (c *Client) DownloadFile(ctx context.Context, url, localPath string) error {
	// Check if we need to update the file
	if c.config.CheckChanges {
		remoteInfo, err := c.CheckFileInfo(ctx, url)
		if err != nil {
			return fmt.Errorf("failed to check remote file info: %w", err)
		}

		needsUpdate, err := c.NeedsUpdate(localPath, remoteInfo)
		if err != nil {
			return fmt.Errorf("failed to check if file needs update: %w", err)
		}

		if !needsUpdate {
			return nil // File is up to date
		}
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create/open the local file
	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	// Make the request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create GET request: %w", err)
	}

	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("GET request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET request returned status %d", resp.StatusCode)
	}

	// Copy with rate limiting
	reader := resp.Body
	if c.limiter != nil {
		reader = &rateLimitedReader{
			reader:  resp.Body,
			limiter: c.limiter,
			ctx:     ctx,
		}
	}

	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Set modification time if available
	if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
		if t, err := time.Parse(time.RFC1123, lastModified); err == nil {
			os.Chtimes(localPath, t, t)
		}
	}

	return nil
}

// rateLimitedReader implements rate limiting for io.Reader
type rateLimitedReader struct {
	reader  io.ReadCloser
	limiter *rate.Limiter
	ctx     context.Context
}

func (r *rateLimitedReader) Read(p []byte) (n int, err error) {
	// Wait for rate limiter
	if err := r.limiter.WaitN(r.ctx, len(p)); err != nil {
		return 0, err
	}

	return r.reader.Read(p)
}

func (r *rateLimitedReader) Close() error {
	return r.reader.Close()
}

// parseRateLimit parses a rate limit string like "500k" into bytes per second
func parseRateLimit(rateStr string) int64 {
	rateStr = strings.ToLower(strings.TrimSpace(rateStr))

	var multiplier int64 = 1
	var numStr string

	if strings.HasSuffix(rateStr, "k") {
		multiplier = 1024
		numStr = strings.TrimSuffix(rateStr, "k")
	} else if strings.HasSuffix(rateStr, "m") {
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(rateStr, "m")
	} else if strings.HasSuffix(rateStr, "g") {
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(rateStr, "g")
	} else {
		numStr = rateStr
	}

	if num, err := strconv.ParseInt(numStr, 10, 64); err == nil {
		return num * multiplier
	}

	return 0
}
