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
	BLE      BLEConfig      `yaml:"ble"`
	Omni     OmniConfig     `yaml:"omni"`
}

type OmniConfig struct {
	Enabled          bool        `yaml:"enabled"`
	MinElevation     float64     `yaml:"min_elevation"`
	TLERefreshHrs    int         `yaml:"tle_refresh_hrs"`
	SchedulerEnabled bool        `yaml:"scheduler_enabled"`
	SatDumpBin       string      `yaml:"satdump_bin"`
	DecoderOutputDir string      `yaml:"decoder_output_dir"`
	ACARS            ACARSConfig `yaml:"acars"`
	GOES             GOESConfig  `yaml:"goes"`
	UAT              UATConfig   `yaml:"uat"`
}

// ACARSConfig controls the Inmarsat L-band ACARS decoder subsystem.
type ACARSConfig struct {
	Enabled          bool    `yaml:"enabled"`
	Satellite        string  `yaml:"satellite"`           // e.g. "inmarsat4-f3"
	FrequencyMHz     float64 `yaml:"frequency_mhz"`       // Aero channel center freq
	STDCFrequencyMHz float64 `yaml:"stdc_frequency_mhz"`  // STD-C channel freq
	SampleRate       int     `yaml:"sample_rate"`
	TCPPort          string  `yaml:"tcp_port"`             // rtl_tcp port (distinct from scheduler)
	SyncIntervalMS   int     `yaml:"sync_interval_ms"`     // Platform ingest batch interval
	Pipeline         string  `yaml:"pipeline"`             // SatDump pipeline ID
}

// GOESConfig controls the GOES HRIT geostationary weather imagery decoder.
type GOESConfig struct {
	Enabled           bool              `yaml:"enabled"`
	Satellite         string            `yaml:"satellite"`            // "GOES-16" or "GOES-18"
	FrequencyMHz      float64           `yaml:"frequency_mhz"`       // 1694.1 MHz HRIT downlink
	SampleRate        int               `yaml:"sample_rate"`          // 2400000 (2.4 Msps)
	TCPPort           string            `yaml:"tcp_port"`             // rtl_tcp port (distinct from ACARS)
	Pipeline          string            `yaml:"pipeline"`             // SatDump pipeline ID
	MaxLocalStorageGB int               `yaml:"max_local_storage_gb"` // Rotate local products after N GB
	Products          GOESProductConfig `yaml:"products"`
}

// GOESProductConfig controls which GOES scan modes to decode and upload.
type GOESProductConfig struct {
	FullDisk  GOESProductEntry `yaml:"full_disk"`
	CONUS     GOESProductEntry `yaml:"conus"`
	Mesoscale GOESProductEntry `yaml:"mesoscale"`
}

// GOESProductEntry controls decoding and upload cadence for a single product type.
type GOESProductEntry struct {
	Decode         bool     `yaml:"decode"`
	UploadInterval string   `yaml:"upload_interval"` // e.g. "30m", "15m", "0" (disabled)
	Composites     []string `yaml:"composites"`      // e.g. ["true_color", "ir_enhanced"]
}

// UATConfig controls the 978 MHz UAT decoder subsystem.
type UATConfig struct {
	Enabled        bool       `yaml:"enabled"`
	Dump978Bin     string     `yaml:"dump978_bin"`      // Path to dump978-fa binary
	SDRSerial      string     `yaml:"sdr_serial"`       // Specific SDR serial, or auto-detect
	Gain           int        `yaml:"gain"`             // SDR gain (0-49)
	BiasT          bool       `yaml:"bias_t"`           // Enable bias-T to power inline LNA
	SyncIntervalMS int        `yaml:"sync_interval_ms"` // Platform ingest batch interval
	FISB           FISBConfig `yaml:"fisb"`
}

// FISBConfig controls FIS-B weather product decoding from UAT uplink frames.
type FISBConfig struct {
	Enabled      bool `yaml:"enabled"`
	TextProducts bool `yaml:"text_products"`
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

type BLEConfig struct {
	Enabled          bool   `yaml:"enabled"`
	WindowSeconds    int    `yaml:"window_seconds"`
	AutoPairOnBoot   bool   `yaml:"auto_pair_on_boot"`
	DeviceNamePrefix string `yaml:"device_name_prefix"`
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
			Port:       8888,
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
		BLE: BLEConfig{
			Enabled:          true,
			WindowSeconds:    300,
			AutoPairOnBoot:   true,
			DeviceNamePrefix: "SkyTracker-",
		},
		Omni: OmniConfig{
			Enabled:          true,
			MinElevation:     5.0,
			TLERefreshHrs:    12,
			SchedulerEnabled: true,
			SatDumpBin:       "satdump",
			DecoderOutputDir: "/tmp/skytracker-sat",
			ACARS: ACARSConfig{
				Enabled:          false,
				Satellite:        "inmarsat4-f3",
				FrequencyMHz:     1545.0,
				STDCFrequencyMHz: 1537.0,
				SampleRate:       1200000,
				TCPPort:          "7655",
				SyncIntervalMS:   5000,
				Pipeline:         "inmarsat_aero_6",
			},
			GOES: GOESConfig{
				Enabled:           false,
				Satellite:         "GOES-18",
				FrequencyMHz:      1694.1,
				SampleRate:        2400000,
				TCPPort:           "7656",
				Pipeline:          "goes_hrit",
				MaxLocalStorageGB: 2,
				Products: GOESProductConfig{
					FullDisk: GOESProductEntry{
						Decode:         true,
						UploadInterval: "30m",
						Composites:     []string{"true_color", "ir_enhanced"},
					},
					CONUS: GOESProductEntry{
						Decode:         true,
						UploadInterval: "15m",
						Composites:     []string{"true_color", "ir_enhanced", "water_vapor"},
					},
					Mesoscale: GOESProductEntry{
						Decode:         true,
						UploadInterval: "0",
						Composites:     []string{"true_color"},
					},
				},
			},
			UAT: UATConfig{
				Enabled:        false,
				Dump978Bin:     "dump978-fa",
				Gain:           48,
				SyncIntervalMS: 10000,
				FISB:           FISBConfig{Enabled: true, TextProducts: true},
			},
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
