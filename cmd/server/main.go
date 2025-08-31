package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jhofer-cloud/http-mirror/pkg/config"
	"github.com/jhofer-cloud/http-mirror/pkg/files"
)

func main() {
	// Parse command line flags
	configFile := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// Setup logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Set config file if provided
	if *configFile != "" {
		os.Setenv("CONFIG_FILE", *configFile)
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger.Info("Starting HTTP Mirror Server", 
		"port", cfg.Server.Port,
		"data_path", cfg.Server.DataPath)

	// Create file handler
	fileHandler, err := files.NewHandler(cfg.Server.DataPath, cfg)
	if err != nil {
		logger.Error("Failed to create file handler", "error", err)
		os.Exit(1)
	}

	// Create HTTP server
	mux := http.NewServeMux()
	
	// File serving handler
	mux.Handle("/", fileHandler)
	
	// Health check endpoint
	mux.HandleFunc("/health", healthCheckHandler)
	
	// Status endpoint
	mux.HandleFunc("/status", statusHandler(cfg))

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: mux,
		
		// Security settings
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info("Server starting", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Server shutting down...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("Server stopped")
}

// healthCheckHandler handles health check requests
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"healthy","service":"http-mirror-server"}`)
}

// statusHandler returns server status information
func statusHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		
		// Get basic statistics about the data directory
		stats, err := getDirStats(cfg.Server.DataPath)
		if err != nil {
			stats = map[string]interface{}{"error": err.Error()}
		}
		
		response := map[string]interface{}{
			"status":    "running",
			"service":   "http-mirror-server",
			"data_path": cfg.Server.DataPath,
			"stats":     stats,
		}
		
		// Simple JSON encoding
		fmt.Fprintf(w, `{
			"status": "%s",
			"service": "%s", 
			"data_path": "%s",
			"stats": %v
		}`, response["status"], response["service"], response["data_path"], stats)
	}
}

// getDirStats returns basic statistics about a directory
func getDirStats(dirPath string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Check if directory exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return stats, fmt.Errorf("directory does not exist: %s", dirPath)
	}
	
	// Count files and directories
	fileCount := 0
	dirCount := 0
	var totalSize int64
	
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if info.IsDir() {
			dirCount++
		} else {
			fileCount++
			totalSize += info.Size()
		}
		
		return nil
	})
	
	if err != nil {
		return stats, err
	}
	
	stats["files"] = fileCount
	stats["directories"] = dirCount
	stats["total_size_bytes"] = totalSize
	
	return stats, nil
}