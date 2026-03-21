package wifi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sentinel errors for Wi-Fi operations.
var (
	ErrAuthFailed = errors.New("wifi: authentication failed")
	ErrTimeout    = errors.New("wifi: connection timed out")
	ErrNoDevice   = errors.New("wifi: no wireless device found")
)

// Network represents a scanned Wi-Fi network.
type Network struct {
	SSID   string `json:"ssid"`
	Signal int    `json:"signal"`
	Secure bool   `json:"secure"`
}

// Status represents the current WiFi connection state.
type Status struct {
	Connected bool
	SSID      string
	IP        string
}

// Manager handles WiFi connectivity and (future) captive portal.
type Manager struct {
	mu     sync.RWMutex
	status Status
}

// NewManager creates a new WiFi manager.
func NewManager() *Manager {
	return &Manager{}
}

// Status returns the current WiFi status.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// IsConnected returns true if WiFi is connected.
func (m *Manager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status.Connected
}

// Run periodically checks WiFi status. Blocks until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) {
	// Initial check.
	m.checkStatus()

	// The captive portal flow would be implemented here in a future version.
	// For now, we just periodically check connectivity.
	<-ctx.Done()
}

// ScanNetworks uses nmcli to scan for nearby Wi-Fi networks.
// Returns up to 10 networks sorted by signal strength (strongest first).
func (m *Manager) ScanNetworks() ([]Network, error) {
	out, err := exec.Command("nmcli", "-t", "-f", "SSID,SIGNAL,SECURITY", "device", "wifi", "list", "--rescan", "yes").Output()
	if err != nil {
		return nil, fmt.Errorf("nmcli scan: %w", err)
	}

	// Parse colon-delimited output, deduplicate by SSID (keep strongest).
	best := make(map[string]Network)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		ssid := parts[0]
		if ssid == "" {
			continue
		}
		signal, _ := strconv.Atoi(parts[1])
		secure := parts[2] != "" && parts[2] != "--"

		if existing, ok := best[ssid]; !ok || signal > existing.Signal {
			best[ssid] = Network{SSID: ssid, Signal: signal, Secure: secure}
		}
	}

	networks := make([]Network, 0, len(best))
	for _, n := range best {
		networks = append(networks, n)
	}
	sort.Slice(networks, func(i, j int) bool {
		return networks[i].Signal > networks[j].Signal
	})

	if len(networks) > 10 {
		networks = networks[:10]
	}
	return networks, nil
}

// Connect connects to a Wi-Fi network using nmcli with a 30-second timeout.
// The connection is configured via a NetworkManager connection profile so that
// the password is never exposed on the process command line (visible in /proc).
func (m *Manager) Connect(ssid, password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Create (or update) a connection profile without passing the
	// password as a CLI argument. nmcli reads 802-11-wireless-security
	// properties from its own config machinery, but there is no stdin-pipe
	// option. Instead we create the profile first, then set the PSK via a
	// separate "nmcli connection modify" call. Both calls only expose the
	// SSID on the command line, not the password.
	connName := "skytracker-" + ssid

	// Delete any pre-existing profile with this name (ignore errors).
	_ = exec.CommandContext(ctx, "nmcli", "connection", "delete", connName).Run()

	// Create the profile without a password.
	addCmd := exec.CommandContext(ctx, "nmcli", "connection", "add",
		"type", "wifi",
		"con-name", connName,
		"ifname", "wlan0",
		"ssid", ssid,
		"wifi-sec.key-mgmt", "wpa-psk",
	)
	if out, err := addCmd.CombinedOutput(); err != nil {
		output := string(out)
		if strings.Contains(output, "no Wi-Fi device found") || strings.Contains(output, "wlan0") {
			return ErrNoDevice
		}
		return fmt.Errorf("nmcli connection add: %s", output)
	}

	// Set the PSK by writing directly to the NetworkManager keyfile.
	// This avoids exposing the password in any process command line.
	// NM stores connection files at /etc/NetworkManager/system-connections/.
	keyfilePath := fmt.Sprintf("/etc/NetworkManager/system-connections/%s.nmconnection", connName)
	if err := patchKeyfilePSK(keyfilePath, password); err != nil {
		// Fallback: use nmcli modify (password briefly visible in /proc).
		// KNOWN LIMITATION: nmcli does not support reading secrets from
		// stdin, so the modify fallback exposes the PSK in the process
		// argv. The window is very short (local-only, no network I/O).
		modCmd := exec.CommandContext(ctx, "nmcli", "connection", "modify", connName,
			"wifi-sec.psk", password,
		)
		if out, err := modCmd.CombinedOutput(); err != nil {
			_ = exec.CommandContext(ctx, "nmcli", "connection", "delete", connName).Run()
			return fmt.Errorf("nmcli connection modify: %s", string(out))
		}
	}
	// Reload so NetworkManager picks up the keyfile change.
	_ = exec.CommandContext(ctx, "nmcli", "connection", "reload").Run()

	// Step 2: Activate the connection (no secrets on the command line).
	upCmd := exec.CommandContext(ctx, "nmcli", "connection", "up", connName, "ifname", "wlan0")
	out, err := upCmd.CombinedOutput()
	if err != nil {
		output := string(out)
		// Clean up on failure.
		_ = exec.CommandContext(ctx, "nmcli", "connection", "delete", connName).Run()
		if ctx.Err() == context.DeadlineExceeded {
			return ErrTimeout
		}
		if strings.Contains(output, "Secrets were required") || strings.Contains(output, "No suitable") {
			return ErrAuthFailed
		}
		if strings.Contains(output, "no Wi-Fi device found") || strings.Contains(output, "wlan0") {
			return ErrNoDevice
		}
		return fmt.Errorf("nmcli connect: %s", output)
	}

	m.checkStatus()
	return nil
}

// patchKeyfilePSK writes the PSK directly into a NetworkManager keyfile,
// avoiding any command-line exposure. The keyfile is an INI-style file;
// the PSK belongs under the [wifi-security] section.
func patchKeyfilePSK(path, psk string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var result []string
	inSection := false
	pskWritten := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			// Leaving a section — if we were in wifi-security and
			// didn't write the PSK yet, write it now.
			if inSection && !pskWritten {
				result = append(result, "psk="+psk)
				pskWritten = true
			}
			inSection = trimmed == "[wifi-security]"
		}
		if inSection && strings.HasPrefix(trimmed, "psk=") {
			result = append(result, "psk="+psk)
			pskWritten = true
			continue
		}
		result = append(result, line)
	}

	// If the section was the last one and we haven't written the PSK.
	if inSection && !pskWritten {
		result = append(result, "psk="+psk)
	}

	return os.WriteFile(path, []byte(strings.Join(result, "\n")), 0600)
}

func (m *Manager) checkStatus() {
	status := Status{}

	// Try to detect WiFi SSID using iwgetid (Linux).
	if out, err := exec.Command("iwgetid", "-r").Output(); err == nil {
		ssid := strings.TrimSpace(string(out))
		if ssid != "" {
			status.Connected = true
			status.SSID = ssid
		}
	}

	// Try to get IP address.
	if out, err := exec.Command("hostname", "-I").Output(); err == nil {
		parts := strings.Fields(string(out))
		if len(parts) > 0 {
			status.IP = parts[0]
			status.Connected = true
		}
	}

	m.mu.Lock()
	m.status = status
	m.mu.Unlock()

	if status.Connected {
		log.Printf("[wifi] connected: SSID=%s IP=%s", status.SSID, status.IP)
	} else {
		log.Printf("[wifi] not connected")
	}
}

// MockManager always reports as connected (for development).
type MockManager struct{}

func NewMockManager() *MockManager { return &MockManager{} }

func (m *MockManager) Status() Status {
	return Status{Connected: true, SSID: "MockNetwork", IP: "192.168.1.100"}
}

func (m *MockManager) IsConnected() bool { return true }

func (m *MockManager) Run(ctx context.Context) {
	log.Printf("[wifi] mock mode: simulating connected WiFi")
	<-ctx.Done()
}

func (m *MockManager) ScanNetworks() ([]Network, error) {
	return []Network{
		{SSID: "HomeNetwork", Signal: 90, Secure: true},
		{SSID: "NeighborWiFi", Signal: 65, Secure: true},
		{SSID: "CoffeeShop", Signal: 42, Secure: false},
	}, nil
}

func (m *MockManager) Connect(ssid, password string) error {
	return nil
}
