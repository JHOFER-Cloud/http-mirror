package files

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jhofer-cloud/http-mirror/pkg/config"
)

// FileInfo represents a file or directory
type FileInfo struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// DirectoryListing represents a directory with its files
type DirectoryListing struct {
	Path        string
	Parent      string
	Files       []FileInfo
	Timestamp   time.Time
	OriginalURL string
	TargetName  string
}

// Handler handles file serving and directory listing
type Handler struct {
	rootPath string
	template *template.Template
	config   *config.Config
}

// NewHandler creates a new file handler
func NewHandler(rootPath string, cfg *config.Config) (*Handler, error) {
	// Ensure root path exists
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	// Parse the directory listing template with custom functions
	tmpl, err := template.New("directory").Funcs(template.FuncMap{
		"formatSize": formatSize,
	}).Parse(directoryTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	return &Handler{
		rootPath: rootPath,
		template: tmpl,
		config:   cfg,
	}, nil
}

// ServeHTTP implements http.Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Clean the URL path
	urlPath := strings.TrimPrefix(r.URL.Path, "/")
	if urlPath == "" {
		urlPath = "."
	}

	// Construct the file path
	filePath := filepath.Join(h.rootPath, urlPath)

	// Security check - ensure path is within root
	cleanPath, err := filepath.Abs(filePath)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	rootAbs, err := filepath.Abs(h.rootPath)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	if !strings.HasPrefix(cleanPath, rootAbs) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// Check if file/directory exists
	stat, err := os.Stat(cleanPath)
	if os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// If it's a file, serve it
	if !stat.IsDir() {
		h.serveFile(w, r, cleanPath)
		return
	}

	// If it's a directory, check for index.html first
	indexPath := filepath.Join(cleanPath, "index.html")
	indexAbsPath, err := filepath.Abs(indexPath)
	if err == nil && strings.HasPrefix(indexAbsPath, rootAbs) {
		if _, err := os.Stat(indexAbsPath); err == nil {
			h.serveFile(w, r, indexAbsPath)
			return
		}
	}

	// Generate directory listing
	h.serveDirectory(w, r, cleanPath, urlPath)
}

// serveFile serves a static file
func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Get file info for headers
	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "Failed to stat file", http.StatusInternalServerError)
		return
	}

	// Set content type based on file extension
	contentType := getContentType(filepath.Ext(filePath))
	w.Header().Set("Content-Type", contentType)

	// Set cache headers
	w.Header().Set("Last-Modified", stat.ModTime().UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Check if-modified-since header
	if modSince := r.Header.Get("If-Modified-Since"); modSince != "" {
		if t, err := time.Parse(http.TimeFormat, modSince); err == nil {
			if stat.ModTime().Before(t.Add(1 * time.Second)) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
	}

	// Serve the file
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
}

// serveDirectory serves a directory listing
func (h *Handler) serveDirectory(w http.ResponseWriter, r *http.Request, dirPath, urlPath string) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, "Failed to read directory", http.StatusInternalServerError)
		return
	}

	// Build file list
	var fileList []FileInfo
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			continue
		}

		// Skip hidden files
		if strings.HasPrefix(file.Name(), ".") {
			continue
		}

		fileList = append(fileList, FileInfo{
			Name:    file.Name(),
			Path:    filepath.Join(urlPath, file.Name()),
			IsDir:   file.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	// Sort files (directories first, then alphabetically)
	sort.Slice(fileList, func(i, j int) bool {
		if fileList[i].IsDir != fileList[j].IsDir {
			return fileList[i].IsDir
		}
		return fileList[i].Name < fileList[j].Name
	})

	// Calculate parent path
	parent := ""
	if urlPath != "." && urlPath != "" {
		// Remove trailing slash for consistent behavior
		cleanPath := strings.TrimSuffix(urlPath, "/")
		parent = filepath.Dir(cleanPath)
		if parent == "." {
			parent = ""
		}
	}

	// Find the target info for this path
	var originalURL, targetName string
	if h.config != nil {
		// Determine which target this path belongs to by checking the first path segment
		pathParts := strings.Split(strings.Trim(urlPath, "/"), "/")
		if len(pathParts) > 0 && pathParts[0] != "" {
			targetName = pathParts[0]
			// Find the corresponding target configuration
			for _, target := range h.config.Targets {
				if target.Name == targetName {
					originalURL = target.URL
					break
				}
			}
		}
	}

	// Create directory listing
	listing := DirectoryListing{
		Path:        urlPath,
		Parent:      parent,
		Files:       fileList,
		Timestamp:   time.Now(),
		OriginalURL: originalURL,
		TargetName:  targetName,
	}

	// Set headers
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// Render template
	if err := h.template.Execute(w, listing); err != nil {
		http.Error(w, "Failed to render directory listing", http.StatusInternalServerError)
		return
	}
}

// getContentType returns the MIME type based on file extension
func getContentType(ext string) string {
	switch strings.ToLower(ext) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".xml":
		return "application/xml; charset=utf-8"
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".zip":
		return "application/zip"
	case ".tar":
		return "application/x-tar"
	case ".gz":
		return "application/gzip"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".md":
		return "text/markdown; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// formatSize formats file size in human readable format
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Directory listing HTML template
const directoryTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Index of {{if .Path}}/{{.Path}}{{else}}/{{end}}</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            margin: 40px;
            background-color: #f5f5f5;
            line-height: 1.6;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        h1 {
            color: #333;
            border-bottom: 2px solid #007acc;
            padding-bottom: 10px;
            margin-bottom: 30px;
        }
        .breadcrumb {
            margin-bottom: 20px;
            font-size: 14px;
            color: #666;
        }
        .breadcrumb a {
            color: #007acc;
            text-decoration: none;
        }
        .breadcrumb a:hover {
            text-decoration: underline;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
        }
        th, td {
            text-align: left;
            padding: 12px;
            border-bottom: 1px solid #eee;
        }
        th {
            background-color: #f8f9fa;
            font-weight: 600;
            color: #555;
        }
        tr:hover {
            background-color: #f8f9fa;
        }
        .icon {
            width: 20px;
            height: 20px;
            margin-right: 8px;
            vertical-align: middle;
        }
        .file-name {
            display: flex;
            align-items: center;
        }
        .file-name a {
            color: #333;
            text-decoration: none;
        }
        .file-name a:hover {
            color: #007acc;
            text-decoration: underline;
        }
        .directory {
            color: #007acc !important;
        }
        .size {
            text-align: right;
            font-family: 'Monaco', 'Menlo', monospace;
            font-size: 13px;
        }
        .date {
            font-family: 'Monaco', 'Menlo', monospace;
            font-size: 13px;
            color: #666;
        }
        .footer {
            margin-top: 40px;
            padding-top: 20px;
            border-top: 1px solid #eee;
            font-size: 12px;
            color: #999;
            text-align: center;
        }
        .footer a {
            color: #007acc;
            text-decoration: none;
        }
        .footer a:hover {
            text-decoration: underline;
        }
        .parent-link {
            margin-bottom: 20px;
        }
        .parent-link a {
            display: inline-block;
            padding: 8px 16px;
            background-color: #007acc;
            color: white;
            text-decoration: none;
            border-radius: 4px;
            font-size: 14px;
        }
        .parent-link a:hover {
            background-color: #005999;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Index of {{if .Path}}/{{.Path}}{{else}}/{{end}}</h1>
        
        {{if .Parent}}
        <div class="parent-link">
            <a href="/{{.Parent}}">📁 Parent Directory</a>
        </div>
        {{end}}

        <table>
            <thead>
                <tr>
                    <th>Name</th>
                    <th>Size</th>
                    <th>Last Modified</th>
                </tr>
            </thead>
            <tbody>
                {{range .Files}}
                <tr>
                    <td class="file-name">
                        {{if .IsDir}}
                        <span class="icon">📁</span>
                        <a href="/{{.Path}}/" class="directory">{{.Name}}/</a>
                        {{else}}
                        <span class="icon">📄</span>
                        <a href="/{{.Path}}">{{.Name}}</a>
                        {{end}}
                    </td>
                    <td class="size">
                        {{if .IsDir}}-{{else}}{{.Size | formatSize}}{{end}}
                    </td>
                    <td class="date">{{.ModTime.Format "2006-01-02 15:04:05"}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>

        <div class="footer">
            {{if .OriginalURL}}
            Mirrored from <a href="{{.OriginalURL}}" target="_blank">{{.OriginalURL}}</a> • {{.Timestamp.Format "2006-01-02 15:04:05"}}
            {{else}}
            Generated by HTTP Mirror • {{.Timestamp.Format "2006-01-02 15:04:05"}}
            {{end}}
        </div>
    </div>
</body>
</html>`

