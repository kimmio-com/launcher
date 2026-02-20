package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BuildMode       string
	DataDir         string
	ListenPort      int
	PortSearchRange int
	MaxProfiles     int
	ActionTimeout   time.Duration
	EnableTimeout   time.Duration
	ProfilePortMin  int
	ProfilePortMax  int
}

func Load(buildMode string) Config {
	cfg := Config{
		BuildMode:       strings.TrimSpace(buildMode),
		ListenPort:      envInt("KIMMIO_PORT", 7331),
		PortSearchRange: envInt("KIMMIO_PORT_SEARCH_RANGE", 100),
		MaxProfiles:     envInt("KIMMIO_MAX_PROFILES", 3),
		ActionTimeout:   envDuration("KIMMIO_ACTION_TIMEOUT", 2*time.Minute),
		EnableTimeout:   envDuration("KIMMIO_ENABLE_TIMEOUT", 20*time.Minute),
		ProfilePortMin:  envInt("KIMMIO_PROFILE_PORT_MIN", 8080),
		ProfilePortMax:  envInt("KIMMIO_PROFILE_PORT_MAX", 9000),
	}
	cfg.DataDir = resolveDataDir(cfg.BuildMode)
	if custom := strings.TrimSpace(os.Getenv("KIMMIO_DATA_DIR")); custom != "" {
		cfg.DataDir = custom
	}
	if cfg.MaxProfiles < 1 {
		cfg.MaxProfiles = 1
	}
	if cfg.ProfilePortMin < 1024 {
		cfg.ProfilePortMin = 1024
	}
	if cfg.ProfilePortMax <= cfg.ProfilePortMin {
		cfg.ProfilePortMax = cfg.ProfilePortMin + 1000
	}
	if cfg.EnableTimeout < cfg.ActionTimeout {
		cfg.EnableTimeout = cfg.ActionTimeout
	}
	return cfg
}

func resolveDataDir(buildMode string) string {
	if buildMode != "prod" {
		return "data"
	}
	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil || strings.TrimSpace(home) == "" {
			return "data"
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "KimmioLauncher")
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}
