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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/jhofer-cloud/http-mirror/pkg/config"
	"github.com/jhofer-cloud/http-mirror/pkg/files"
)

var (
	// Metrics for the mirror server
	mirrorFilesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "http_mirror_files_total",
			Help: "Total number of mirrored files",
		},
		[]string{"target", "data_path"},
	)
	mirrorDirectoriesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "http_mirror_directories_total", 
			Help: "Total number of mirrored directories",
		},
		[]string{"target", "data_path"},
	)
	mirrorSizeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "http_mirror_size_bytes",
			Help: "Total size of mirrored data in bytes",
		},
		[]string{"target", "data_path"},
	)
)

func main() {
	// Register Prometheus metrics
	prometheus.MustRegister(mirrorFilesTotal)
	prometheus.MustRegister(mirrorDirectoriesTotal) 
	prometheus.MustRegister(mirrorSizeBytes)

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
	
	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Initialize metrics immediately
	updateMetrics(cfg, logger)
	
	// Start metrics updater
	go updateMetricsLoop(cfg, logger)

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

// updateMetricsLoop periodically updates Prometheus metrics
func updateMetricsLoop(cfg *config.Config, logger *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			updateMetrics(cfg, logger)
		}
	}
}

// updateMetrics calculates and updates Prometheus metrics
func updateMetrics(cfg *config.Config, logger *slog.Logger) {
	// Update global metrics for the entire data path
	globalStats, err := getDirStats(cfg.Server.DataPath)
	if err != nil {
		logger.Warn("Failed to update global metrics", "error", err)
	} else {
		dataPath := cfg.Server.DataPath
		
		if files, ok := globalStats["files"].(int); ok {
			mirrorFilesTotal.WithLabelValues("_global", dataPath).Set(float64(files))
		}
		
		if dirs, ok := globalStats["directories"].(int); ok {
			mirrorDirectoriesTotal.WithLabelValues("_global", dataPath).Set(float64(dirs))
		}
		
		if size, ok := globalStats["total_size_bytes"].(int64); ok {
			mirrorSizeBytes.WithLabelValues("_global", dataPath).Set(float64(size))
		}
	}

	// Update per-target metrics
	for _, target := range cfg.Targets {
		targetPath := filepath.Join(cfg.Server.DataPath, target.Name)
		targetStats, err := getDirStats(targetPath)
		if err != nil {
			logger.Warn("Failed to update target metrics", "target", target.Name, "error", err)
			// Set zero values for missing targets
			mirrorFilesTotal.WithLabelValues(target.Name, targetPath).Set(0)
			mirrorDirectoriesTotal.WithLabelValues(target.Name, targetPath).Set(0)
			mirrorSizeBytes.WithLabelValues(target.Name, targetPath).Set(0)
			continue
		}
		
		if files, ok := targetStats["files"].(int); ok {
			mirrorFilesTotal.WithLabelValues(target.Name, targetPath).Set(float64(files))
		}
		
		if dirs, ok := targetStats["directories"].(int); ok {
			mirrorDirectoriesTotal.WithLabelValues(target.Name, targetPath).Set(float64(dirs))
		}
		
		if size, ok := targetStats["total_size_bytes"].(int64); ok {
			mirrorSizeBytes.WithLabelValues(target.Name, targetPath).Set(float64(size))
		}
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