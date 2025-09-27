package mirror

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jhofer-cloud/http-mirror/pkg/config"
	httpPkg "github.com/jhofer-cloud/http-mirror/pkg/http"
)

// Manager handles the mirroring process
type Manager struct {
	config *config.Config
	logger *slog.Logger
}

// NewManager creates a new mirror manager
func NewManager(cfg *config.Config, logger *slog.Logger) *Manager {
	return &Manager{
		config: cfg,
		logger: logger,
	}
}

// MirrorTarget mirrors a single target
func (m *Manager) MirrorTarget(ctx context.Context, target *config.Target) error {
	m.logger.Info("Starting mirror for target", "name", target.Name, "url", target.URL)

	// Create HTTP client for this target
	client := httpPkg.NewClient(target)

	// Create target directory
	targetDir := filepath.Join(m.config.Mirror.DataPath, target.Name)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Start mirroring from the root URL
	stats := &MirrorStats{
		StartTime: time.Now(),
		Target:    target.Name,
	}

	err := m.mirrorURL(ctx, client, target, target.URL, targetDir, 0, stats)

	stats.EndTime = time.Now()
	stats.Duration = stats.EndTime.Sub(stats.StartTime)

	m.logger.Info("Mirror completed for target",
		"name", target.Name,
		"duration", stats.Duration,
		"files_downloaded", stats.FilesDownloaded,
		"files_skipped", stats.FilesSkipped,
		"bytes_downloaded", stats.BytesDownloaded,
		"errors", stats.Errors)

	return err
}

// MirrorStats tracks mirroring statistics
type MirrorStats struct {
	StartTime       time.Time
	EndTime         time.Time
	Duration        time.Duration
	Target          string
	FilesDownloaded int64
	FilesSkipped    int64
	BytesDownloaded int64
	Errors          int64
}

// mirrorURL recursively mirrors a URL and its contents
func (m *Manager) mirrorURL(ctx context.Context, client *httpPkg.Client, target *config.Target,
	currentURL, localDir string, depth int, stats *MirrorStats,
) error {
	// Check depth limit (-1 means unlimited)
	if target.MaxDepth >= 0 && depth >= target.MaxDepth {
		return nil
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Wait between requests if configured
	if depth > 0 && target.WaitBetweenRequests > 0 {
		time.Sleep(target.GetWaitDuration())
	}

	m.logger.Debug("Processing URL", "url", currentURL, "depth", depth)

	// Parse the URL
	parsedURL, err := url.Parse(currentURL)
	if err != nil {
		stats.Errors++
		return fmt.Errorf("failed to parse URL %s: %w", currentURL, err)
	}

	// Try to get directory listing
	resp, err := m.fetchDirectoryListing(ctx, client, currentURL)
	if err != nil {
		stats.Errors++
		return fmt.Errorf("failed to fetch directory listing from %s: %w", currentURL, err)
	}
	defer resp.Body.Close()

	// Check if this looks like a directory listing
	contentType := resp.Header.Get("Content-Type")
	m.logger.Debug("Fetched URL", "url", currentURL, "contentType", contentType)

	if strings.Contains(contentType, "text/html") {
		// Parse HTML to find links
		links, err := m.parseDirectoryListing(resp, currentURL)
		if err != nil {
			m.logger.Warn("Failed to parse directory listing", "url", currentURL, "error", err)
			stats.Errors++
			return nil
		}

		m.logger.Debug("Parsed directory listing", "url", currentURL, "linkCount", len(links))

		// If no links found, treat as a direct file
		if len(links) == 0 {
			filename := filepath.Base(parsedURL.Path)
			if filename == "" || filename == "." {
				filename = "index.html"
			}
			localPath := filepath.Join(localDir, filename)
			m.logger.Debug("No links found, treating as direct file", "url", currentURL, "filename", filename)
			if err := m.downloadFile(ctx, client, currentURL, localPath, stats); err != nil {
				m.logger.Warn("Failed to download file", "url", currentURL, "error", err)
			}
			return nil
		}

		// Process each link
		for _, link := range links {
			linkURL, err := url.Parse(link)
			if err != nil {
				continue
			}

			// Resolve relative URLs
			absoluteURL := parsedURL.ResolveReference(linkURL).String()

			// Skip parent directory links
			if strings.Contains(link, "..") || strings.Contains(link, "Parent Directory") {
				continue
			}

			// Determine if this is a directory or file
			if strings.HasSuffix(link, "/") {
				// It's a directory - recurse
				dirName := strings.TrimSuffix(link, "/")

				// Security: Validate directory name
				if !isValidFilename(dirName) {
					m.logger.Warn("Skipping invalid directory name", "name", dirName)
					continue
				}

				subDir := filepath.Join(localDir, dirName)

				// Security: Ensure the path stays within bounds
				if !strings.HasPrefix(subDir, localDir) {
					m.logger.Warn("Skipping directory outside bounds", "path", subDir)
					stats.Errors++
					continue
				}

				if err := os.MkdirAll(subDir, 0755); err != nil {
					stats.Errors++
					continue
				}

				if err := m.mirrorURL(ctx, client, target, absoluteURL, subDir, depth+1, stats); err != nil {
					m.logger.Warn("Failed to mirror subdirectory", "url", absoluteURL, "error", err)
				}
			} else {
				// It's a file - download it
				filename := filepath.Base(link)

				// Security: Validate filename
				if !isValidFilename(filename) {
					m.logger.Warn("Skipping invalid filename", "name", filename)
					continue
				}

				localPath := filepath.Join(localDir, filename)

				// Security: Ensure the path stays within bounds
				if !strings.HasPrefix(localPath, localDir) {
					m.logger.Warn("Skipping file outside bounds", "path", localPath)
					stats.Errors++
					continue
				}

				if err := m.downloadFile(ctx, client, absoluteURL, localPath, stats); err != nil {
					m.logger.Warn("Failed to download file", "url", absoluteURL, "error", err)
				}
			}
		}
	} else {
		// This is a direct file - download it
		filename := filepath.Base(parsedURL.Path)
		if filename == "" || filename == "." || filename == "/" {
			filename = "index.html"
		}
		localPath := filepath.Join(localDir, filename)
		m.logger.Debug("Downloading direct file", "url", currentURL, "filename", filename, "localPath", localPath)
		if err := m.downloadFile(ctx, client, currentURL, localPath, stats); err != nil {
			m.logger.Warn("Failed to download file", "url", currentURL, "error", err)
		}
	}

	return nil
}

// fetchDirectoryListing fetches a directory listing
func (m *Manager) fetchDirectoryListing(ctx context.Context, client *httpPkg.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", client.GetUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	return client.DoRequest(req)
}

// parseDirectoryListing parses HTML directory listing to extract links
func (m *Manager) parseDirectoryListing(resp *http.Response, baseURL string) ([]string, error) {
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	content := string(body)
	var links []string

	// Pattern to match href links in directory listings
	// This is a simple regex - could be improved with proper HTML parsing
	linkPattern := regexp.MustCompile(`href=["']([^"']+)["']`)
	matches := linkPattern.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) > 1 {
			link := match[1]

			// Skip certain links (security: prevent various types of malicious links)
			if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") ||
				strings.HasPrefix(link, "mailto:") || strings.HasPrefix(link, "ftp://") ||
				strings.HasPrefix(link, "javascript:") || strings.HasPrefix(link, "data:") || strings.HasPrefix(link, "vbscript:") ||
				strings.HasPrefix(link, "#") || link == "/" || link == "./" {
				continue
			}

			// Security: Comprehensive path traversal prevention
			if strings.Contains(link, "..") ||
				strings.HasPrefix(link, "../") ||
				strings.Contains(link, "/..") ||
				strings.HasSuffix(link, "/..") ||
				link == ".." {
				continue
			}

			// Skip common non-content links
			if strings.Contains(strings.ToLower(link), "parent") ||
				strings.Contains(strings.ToLower(link), "back") ||
				strings.Contains(link, "?") || strings.Contains(link, "&") {
				continue
			}

			// Security: Skip empty or suspicious links
			if link == "" || strings.TrimSpace(link) == "" {
				continue
			}

			links = append(links, link)
		}
	}

	return links, nil
}

// isValidFilename checks if a filename is safe for mirroring (minimal filtering for old file compatibility)
func isValidFilename(filename string) bool {
	// Reject empty names
	if filename == "" || strings.TrimSpace(filename) == "" {
		return false
	}

	// Only reject clear path traversal attempts - keep it minimal for old files
	if strings.Contains(filename, "..") ||
		strings.Contains(filename, "/") ||
		strings.Contains(filename, "\\") {
		return false
	}

	return true
}

// downloadFile downloads a single file
func (m *Manager) downloadFile(ctx context.Context, client *httpPkg.Client, url, localPath string, stats *MirrorStats) error {
	// Check if file needs updating
	if client.GetConfig().CheckChanges {
		remoteInfo, err := client.CheckFileInfo(ctx, url)
		if err != nil {
			// If we can't check, try to download anyway
			m.logger.Debug("Could not check file info, downloading anyway", "url", url, "error", err)
		} else {
			needsUpdate, err := client.NeedsUpdate(localPath, remoteInfo)
			if err != nil {
				m.logger.Debug("Could not check if file needs update, downloading anyway", "path", localPath, "error", err)
			} else if !needsUpdate {
				m.logger.Debug("File is up to date, skipping", "path", localPath)
				stats.FilesSkipped++
				return nil
			}
		}
	}

	m.logger.Debug("Downloading file", "url", url, "path", localPath)

	// Download the file
	err := client.DownloadFile(ctx, url, localPath)
	if err != nil {
		stats.Errors++
		return err
	}

	// Update stats
	if stat, err := os.Stat(localPath); err == nil {
		stats.BytesDownloaded += stat.Size()
	}
	stats.FilesDownloaded++

	return nil
}
