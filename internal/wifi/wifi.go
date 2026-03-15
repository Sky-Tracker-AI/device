package wifi

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"sync"
)

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
