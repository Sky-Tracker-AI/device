package wifi

import (
	"context"
	"errors"
	"fmt"
	"log"
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
func (m *Manager) Connect(ssid, password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nmcli", "device", "wifi", "connect", ssid, "password", password, "ifname", "wlan0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := string(out)
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
