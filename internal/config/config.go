// Package config reads everything from the environment so compose is the only place
// where this service is configured.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/PlatanosVerdes/wallapop-reactivator/internal/wallapop"
)

type Config struct {
	DataDir    string
	BaseURL    string
	WebURL     string
	Scheme     wallapop.SignScheme
	DeviceID   string
	AppVersion string

	Interval time.Duration
	Port     int

	MinPause  time.Duration
	MaxPause  time.Duration
	MaxPerRun int

	// Pushgateway is empty when there is nothing to report to, which is the case
	// outside the Pi.
	Pushgateway string
	WarnBefore  time.Duration

	LogJSON bool
}

func Load() (Config, error) {
	cfg := Config{
		DataDir:     env("WALLA_DATA_DIR", "./data"),
		BaseURL:     env("WALLA_BASE_URL", wallapop.DefaultBaseURL),
		WebURL:      env("WALLA_WEB_URL", wallapop.DefaultWebURL),
		Scheme:      wallapop.SignScheme(env("WALLA_SIGN_SCHEME", string(wallapop.SchemeNone))),
		DeviceID:    env("WALLA_DEVICE_ID", ""),
		AppVersion:  env("WALLA_APP_VERSION", wallapop.DefaultAppVersion),
		Interval:    duration("WALLA_INTERVAL", 24*time.Hour),
		Port:        number("WALLA_PORT", 8000),
		MinPause:    duration("WALLA_MIN_PAUSE", 20*time.Second),
		MaxPause:    duration("WALLA_MAX_PAUSE", 90*time.Second),
		MaxPerRun:   number("WALLA_MAX_PER_RUN", 25),
		Pushgateway: env("WALLA_PUSHGATEWAY", ""),
		WarnBefore:  duration("WALLA_WARN_BEFORE", 72*time.Hour),
		LogJSON:     env("WALLA_LOG_JSON", "") == "1",
	}

	if cfg.MaxPause < cfg.MinPause {
		return cfg, fmt.Errorf("WALLA_MAX_PAUSE (%s) is below WALLA_MIN_PAUSE (%s)", cfg.MaxPause, cfg.MinPause)
	}
	if !wallapop.ValidScheme(cfg.Scheme) {
		return cfg, fmt.Errorf("WALLA_SIGN_SCHEME %q is not one of none, pipe, legacy", cfg.Scheme)
	}

	// A change on their side should be a redeploy, not a rebuild.
	if v := os.Getenv("WALLA_PATH_ITEMS"); v != "" {
		wallapop.PathItems = v
	}
	if v := os.Getenv("WALLA_PATH_REACTIVATE"); v != "" {
		wallapop.PathReactivate = v
	}
	if v := os.Getenv("WALLA_REACTIVATE_METHOD"); v != "" {
		wallapop.ReactivateMethod = v
	}

	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func number(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return v
}

func duration(key string, fallback time.Duration) time.Duration {
	v, err := time.ParseDuration(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return v
}
