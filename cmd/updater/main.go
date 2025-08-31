package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jhofer-cloud/http-mirror/pkg/config"
	"github.com/jhofer-cloud/http-mirror/pkg/mirror"
)

func main() {
	// Parse command line flags
	configFile := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// Setup logging
	logLevel := slog.LevelInfo
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		switch level {
		case "debug":
			logLevel = slog.LevelDebug
		case "warn":
			logLevel = slog.LevelWarn
		case "error":
			logLevel = slog.LevelError
		}
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("Starting HTTP Mirror Updater")

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

	// Validate configuration
	if len(cfg.Targets) == 0 {
		logger.Error("No mirror targets configured")
		os.Exit(1)
	}

	logger.Info("Configuration loaded", 
		"targets", len(cfg.Targets),
		"data_path", cfg.Mirror.DataPath)

	// Create mirror manager
	manager := mirror.NewManager(cfg, logger)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Mirror all targets
	var errors []error
	for i, target := range cfg.Targets {
		logger.Info("Starting mirror for target", 
			"index", i+1,
			"total", len(cfg.Targets),
			"name", target.Name,
			"url", target.URL)

		startTime := time.Now()
		err := manager.MirrorTarget(ctx, &target)
		duration := time.Since(startTime)

		if err != nil {
			logger.Error("Failed to mirror target", 
				"name", target.Name, 
				"url", target.URL,
				"duration", duration,
				"error", err)
			errors = append(errors, fmt.Errorf("target %s: %w", target.Name, err))
		} else {
			logger.Info("Successfully mirrored target", 
				"name", target.Name, 
				"url", target.URL,
				"duration", duration)
		}
	}

	// Final summary
	if len(errors) > 0 {
		logger.Error("Mirror process completed with errors", 
			"successful", len(cfg.Targets)-len(errors),
			"failed", len(errors),
			"total", len(cfg.Targets))
		
		for _, err := range errors {
			logger.Error("Error details", "error", err)
		}
		
		// Exit with error code if any mirrors failed
		os.Exit(1)
	} else {
		logger.Info("Mirror process completed successfully", 
			"targets", len(cfg.Targets))
	}
}