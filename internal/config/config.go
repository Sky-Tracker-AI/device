package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for the SkyTracker Agent.
type Config struct {
	Station  StationConfig  `yaml:"station"`
	Platform PlatformConfig `yaml:"platform"`
	Display  DisplayConfig  `yaml:"display"`
	Sources  SourcesConfig  `yaml:"sources"`
	Advanced AdvancedConfig `yaml:"advanced"`
}

type StationConfig struct {
	Name    string  `yaml:"name"`
	Sharing string  `yaml:"sharing"`
	Lat     float64 `yaml:"lat"`
	Lon     float64 `yaml:"lon"`
}

type PlatformConfig struct {
	APIKey   string `yaml:"api_key"`
	Endpoint string `yaml:"endpoint"`
}

type DisplayConfig struct {
	Port       int             `yaml:"port"`
	Brightness int             `yaml:"brightness"`
	NightMode  NightModeConfig `yaml:"night_mode"`
}

type NightModeConfig struct {
	Enabled bool   `yaml:"enabled"`
	Start   string `yaml:"start"`
	End     string `yaml:"end"`
}

type SourcesConfig struct {
	Dump1090URL string `yaml:"dump1090_url"`
	GPSDHost    string `yaml:"gpsd_host"`
	GPSDPort    int    `yaml:"gpsd_port"`
}

type AdvancedConfig struct {
	PollIntervalMS int `yaml:"poll_interval_ms"`
	MaxRangeNM     int `yaml:"max_range_nm"`
	DataQueueMaxMB int `yaml:"data_queue_max_mb"`
}

// Default returns a Config populated with default values.
func Default() *Config {
	return &Config{
		Station: StationConfig{
			Name:    "SkyTracker",
			Sharing: "private",
		},
		Platform: PlatformConfig{
			Endpoint: "https://api.skytracker.ai",
		},
		Display: DisplayConfig{
			Port:       8080,
			Brightness: 100,
			NightMode: NightModeConfig{
				Enabled: true,
				Start:   "21:00",
				End:     "06:00",
			},
		},
		Sources: SourcesConfig{
			Dump1090URL: "http://localhost/tar1090/data/aircraft.json",
			GPSDHost:    "localhost",
			GPSDPort:    2947,
		},
		Advanced: AdvancedConfig{
			PollIntervalMS: 1000,
			MaxRangeNM:     250,
			DataQueueMaxMB: 100,
		},
	}
}

// Load reads configuration from YAML files, merging user-level overrides
// on top of system-level config, on top of defaults.
// Search order (highest priority first):
//  1. ~/.skytracker/config.yaml
//  2. /etc/skytracker/config.yaml
//  3. Built-in defaults
func Load() (*Config, error) {
	cfg := Default()

	// System-level config.
	if err := mergeFromFile(cfg, "/etc/skytracker/config.yaml"); err != nil {
		return nil, err
	}

	// User-level config (overrides system).
	home, err := os.UserHomeDir()
	if err == nil {
		userPath := filepath.Join(home, ".skytracker", "config.yaml")
		if err := mergeFromFile(cfg, userPath); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

// LoadFromPath loads configuration from a specific file path, merged on top
// of defaults.
func LoadFromPath(path string) (*Config, error) {
	cfg := Default()
	if err := mergeFromFile(cfg, path); err != nil {
		return nil, err
	}
	return cfg, nil
}

// mergeFromFile reads a YAML file and unmarshals it into cfg, overwriting
// only the fields present in the file. If the file does not exist, this is
// a no-op.
func mergeFromFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return yaml.Unmarshal(data, cfg)
}
