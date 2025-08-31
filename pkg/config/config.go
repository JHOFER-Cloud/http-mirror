package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Target represents a single mirror target
type Target struct {
	Name                  string `json:"name"`
	URL                   string `json:"url"`
	UserAgent             string `json:"userAgent,omitempty"`
	RateLimit             string `json:"rateLimit,omitempty"`
	Retries               int    `json:"retries,omitempty"`
	MaxDepth              int    `json:"maxDepth,omitempty"`
	Timeout               int    `json:"timeout,omitempty"`
	WaitBetweenRequests   int    `json:"waitBetweenRequests,omitempty"`
	Timestamping          bool   `json:"timestamping,omitempty"`
	NoClobber             bool   `json:"noClobber,omitempty"`
	ContinueDownload      bool   `json:"continueDownload,omitempty"`
	CheckChanges          bool   `json:"checkChanges,omitempty"`
}

// Config represents the complete mirror configuration
type Config struct {
	Defaults Defaults `json:"defaults"`
	Targets  []Target `json:"targets"`
	Mirror   Mirror   `json:"mirror"`
	Server   Server   `json:"server"`
}

// Defaults contains default values for all targets
type Defaults struct {
	UserAgent           string `json:"userAgent"`
	RateLimit           string `json:"rateLimit"`
	Retries             int    `json:"retries"`
	MaxDepth            int    `json:"maxDepth"`
	Timeout             int    `json:"timeout"`
	WaitBetweenRequests int    `json:"waitBetweenRequests"`
	Timestamping        bool   `json:"timestamping"`
	NoClobber           bool   `json:"noClobber"`
	ContinueDownload    bool   `json:"continueDownload"`
	CheckChanges        bool   `json:"checkChanges"`
}

// Mirror contains mirroring-specific configuration
type Mirror struct {
	DataPath string `json:"dataPath"`
	LogLevel string `json:"logLevel"`
}

// Server contains web server configuration
type Server struct {
	Port    int    `json:"port"`
	Host    string `json:"host"`
	DataPath string `json:"dataPath"`
}

// GetDefaults returns default configuration values
func GetDefaults() Defaults {
	return Defaults{
		UserAgent:           "Mozilla/5.0 (compatible; HttpMirror/1.0; +https://github.com/jhofer-cloud/http-mirror) Friendly Educational Mirror",
		RateLimit:           "500k",
		Retries:             3,
		MaxDepth:            5,
		Timeout:             30,
		WaitBetweenRequests: 1,
		Timestamping:        true,
		NoClobber:           true,
		ContinueDownload:    true,
		CheckChanges:        true,
	}
}

// LoadConfig loads configuration from environment variables and config file
func LoadConfig() (*Config, error) {
	config := &Config{
		Defaults: GetDefaults(),
		Mirror: Mirror{
			DataPath: getEnv("MIRROR_DATA_PATH", "/data"),
			LogLevel: getEnv("LOG_LEVEL", "info"),
		},
		Server: Server{
			Port:     getEnvInt("SERVER_PORT", 8080),
			Host:     getEnv("SERVER_HOST", "0.0.0.0"),
			DataPath: getEnv("SERVER_DATA_PATH", "/data"),
		},
	}

	// Load targets from config file or environment
	if configFile := os.Getenv("CONFIG_FILE"); configFile != "" {
		if err := loadConfigFile(config, configFile); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	} else {
		// Load from environment variables
		loadFromEnv(config)
	}

	// Apply defaults to targets
	for i := range config.Targets {
		applyDefaults(&config.Targets[i], config.Defaults)
	}

	return config, nil
}

// loadConfigFile loads configuration from a JSON file
func loadConfigFile(config *Config, filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, config)
}

// loadFromEnv loads basic configuration from environment variables
func loadFromEnv(config *Config) {
	// Simple single target from env vars
	if url := os.Getenv("MIRROR_URL"); url != "" {
		name := getEnv("MIRROR_NAME", "default")
		config.Targets = []Target{
			{
				Name: name,
				URL:  url,
			},
		}
	}
}

// applyDefaults applies default values to a target if they're not set
func applyDefaults(target *Target, defaults Defaults) {
	if target.UserAgent == "" {
		target.UserAgent = defaults.UserAgent
	}
	if target.RateLimit == "" {
		target.RateLimit = defaults.RateLimit
	}
	if target.Retries == 0 {
		target.Retries = defaults.Retries
	}
	if target.MaxDepth == 0 {
		target.MaxDepth = defaults.MaxDepth
	}
	if target.Timeout == 0 {
		target.Timeout = defaults.Timeout
	}
	if target.WaitBetweenRequests == 0 {
		target.WaitBetweenRequests = defaults.WaitBetweenRequests
	}
	if !target.Timestamping {
		target.Timestamping = defaults.Timestamping
	}
	if !target.NoClobber {
		target.NoClobber = defaults.NoClobber
	}
	if !target.ContinueDownload {
		target.ContinueDownload = defaults.ContinueDownload
	}
	if !target.CheckChanges {
		target.CheckChanges = defaults.CheckChanges
	}
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt gets an integer environment variable with a default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// GetTimeout returns the timeout duration for a target
func (t *Target) GetTimeout() time.Duration {
	return time.Duration(t.Timeout) * time.Second
}

// GetWaitDuration returns the wait duration between requests for a target
func (t *Target) GetWaitDuration() time.Duration {
	return time.Duration(t.WaitBetweenRequests) * time.Second
}