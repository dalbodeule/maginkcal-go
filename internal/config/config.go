package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// NOTE: This file provides the configuration model and full YAML-based
// load/save behavior, including first-run config creation and 0600
// permissions.

// ICSConfig describes a single ICS subscription source.
type ICSConfig struct {
	// URL is the ICS subscription endpoint.
	URL string `yaml:"url" json:"url"`
	// ID is an internal identifier used for de-dup and logging.
	ID string `yaml:"id" json:"id"`
	// Name is a human-friendly label shown in the UI.
	Name string `yaml:"name" json:"name"`
}

// BasicAuthConfig holds HTTP Basic Auth credentials for the Web UI/API.
type BasicAuthConfig struct {
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
}

// Config is the top-level application configuration.
type Config struct {
	// Listen is the HTTP listen address for the Web UI and API.
	Listen string `yaml:"listen" json:"listen"`

	// Timezone is the IANA timezone used as canonical display zone (e.g. "Asia/Seoul").
	Timezone string `yaml:"timezone" json:"timezone"`

	// WeekStart controls which weekday is treated as the first day of the week
	// in calendar views. Supported values:
	//   - "monday" (default)
	//   - "sunday"
	WeekStart string `yaml:"week_start" json:"week_start"`

	// RefreshCron is a cron-style schedule string (e.g. "*/15 * * * *")
	// used for periodic refresh. If empty, it may be derived from
	// RefreshMinutes for backward compatibility.
	RefreshCron string `yaml:"refresh" json:"refresh"`

	// HorizonDays is the number of future days to display.
	HorizonDays int `yaml:"horizon_days" json:"horizon_days"`

	// ShowAllDay toggles the all-day section in the rendered view.
	ShowAllDay bool `yaml:"show_all_day" json:"show_all_day"`

	// HighlightRed is a list of keywords that cause events to be rendered in red.
	HighlightRed []string `yaml:"highlight_red" json:"highlight_red"`

	// ICS is the list of subscribed ICS sources.
	ICS []ICSConfig `yaml:"ics" json:"ics"`

	// BasicAuth, if non-nil, enables HTTP Basic Authentication on all endpoints
	// except /health.
	BasicAuth *BasicAuthConfig `yaml:"basic_auth,omitempty" json:"basic_auth,omitempty"`
}

// DefaultConfig returns an in-memory default configuration.
func DefaultConfig() *Config {
	return &Config{
		Listen:      "127.0.0.1:8080",
		Timezone:    "Asia/Seoul",
		WeekStart:   "monday",
		RefreshCron: "*/15 * * * *",
		// Keep a sensible default for legacy minutes as well, but the
		// application should primarily use RefreshCron.
		HorizonDays:  7,
		ShowAllDay:   true,
		HighlightRed: []string{"휴일", "휴가", "중요"},
		ICS:          []ICSConfig{},
		BasicAuth:    nil,
	}
}

// Normalize fills in missing/zero values with sensible defaults so that
// partially-filled configs (e.g., older versions) still behave correctly.
func (c *Config) Normalize() {
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8080"
	}
	if c.Timezone == "" {
		c.Timezone = "Asia/Seoul"
	}
	// WeekStart default & validation.
	switch c.WeekStart {
	case "monday", "sunday":
		// ok
	case "":
		c.WeekStart = "monday"
	default:
		// Unknown value; fall back to monday to avoid surprising layouts.
		c.WeekStart = "monday"
	}

	// Derive RefreshCron if missing, using RefreshMinutes as a legacy source.
	if c.RefreshCron == "" {
		c.RefreshCron = "*/15 * * * *"
	}
	if c.HorizonDays <= 0 {
		c.HorizonDays = 7
	}
	// ShowAllDay default: true
	// Only treat unset if HighlightRed is nil and we want to ensure a base list.
	if c.HighlightRed == nil {
		c.HighlightRed = []string{"휴일", "휴가", "중요"}
	}
	if c.ICS == nil {
		c.ICS = []ICSConfig{}
	}
}

// Load loads configuration from the given YAML path.
//
// Behavior:
//   - If the file does not exist:
//   - create parent directory if needed
//   - write a default config with 0600 perms
//   - return the default config
//   - If the file exists:
//   - read YAML and unmarshal into Config
//   - normalize defaults
func Load(path string) (*Config, error) {
	if path == "" {
		return nil, errors.New("config path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// First run: create default config file.
			cfg := DefaultConfig()
			if err := Save(path, cfg); err != nil {
				// Even if save fails, return cfg with error so caller can decide.
				return cfg, err
			}
			return cfg, nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.Normalize()

	return &cfg, nil
}

// Save writes the given configuration to the specified path.
//
// Implementation details:
//   - Ensures parent directory exists (0700).
//   - Marshals cfg to YAML.
//   - Writes atomically via a temp file + rename.
//   - Ensures final file permissions are 0600.
func Save(path string, cfg *Config) error {
	if path == "" {
		return errors.New("config path is empty")
	}
	if cfg == nil {
		return errors.New("config is nil")
	}

	cfg.Normalize()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	// Atomic write: write to temp file in same directory then rename.
	tmp, err := os.CreateTemp(dir, ".epdcal-config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	// Ensure we clean up temp file on error.
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}

	// Flush and close before chmod/rename.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Set permissions to 0600 on temp file before rename.
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}

	// Rename over the target path.
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}

	return nil
}

// Save is a convenience method on Config that delegates to the package-level
// Save function. This makes Web UI code slightly nicer:
//
//	cfg, _ := config.Load(path)
//	// ... mutate cfg ...
//	if err := cfg.Save(path); err != nil { ... }
func (c *Config) Save(path string) error {
	return Save(path, c)
}
