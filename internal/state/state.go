package state

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// State holds persistent agent state across restarts.
type State struct {
	Serial    string `json:"serial"`
	DeviceID  string `json:"device_id,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	StationID string `json:"station_id,omitempty"`
	ClaimCode string `json:"claim_code,omitempty"`
	Claimed   bool   `json:"claimed"`

	path string `json:"-"`
}

// Load reads state from the given path. If the file doesn't exist, a new
// state is created with a generated serial.
func Load(path string) (*State, error) {
	// Ensure parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s := &State{
				Serial: GenerateSerial(),
				path:   path,
			}
			if err := s.Save(); err != nil {
				return nil, err
			}
			return s, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}
	s.path = path
	return &s, nil
}

// Save writes the state to disk atomically.
func (s *State) Save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write state tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

// IsRegistered returns true if the agent has registered with the platform.
func (s *State) IsRegistered() bool {
	return s.APIKey != ""
}

// GenerateSerial reads /etc/machine-id if available, otherwise generates
// a random hex string. The result is prefixed with "ST-".
func GenerateSerial() string {
	// Try /etc/machine-id (Linux).
	if data, err := os.ReadFile("/etc/machine-id"); err == nil {
		id := strings.TrimSpace(string(data))
		if len(id) >= 12 {
			return "ST-" + strings.ToUpper(id[:12])
		}
	}

	// Fallback: random hex.
	b := make([]byte, 6)
	rand.Read(b)
	return "ST-" + strings.ToUpper(hex.EncodeToString(b))
}
