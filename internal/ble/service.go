package ble

import (
	"context"

	"github.com/skytracker/skytracker-device/internal/config"
	"github.com/skytracker/skytracker-device/internal/state"
	"github.com/skytracker/skytracker-device/internal/wifi"
)

// WifiManager is the subset of wifi.Manager methods the BLE service needs.
type WifiManager interface {
	ScanNetworks() ([]wifi.Network, error)
	Connect(ssid, password string) error
}

// RegisterFunc is called after Wi-Fi connects to register an unregistered device.
type RegisterFunc func(ctx context.Context) error

// Service implements the BLE GATT peripheral for provisioning.
// On non-Linux platforms, all methods are no-ops.
type Service struct {
	impl serviceImpl

	cfg        config.BLEConfig
	wifiMgr    WifiManager
	agentState *state.State
	version    string
}

// NewService creates a new BLE provisioning service.
func NewService(cfg config.BLEConfig, wifiMgr WifiManager, agentState *state.State, version string) *Service {
	s := &Service{
		cfg:        cfg,
		wifiMgr:    wifiMgr,
		agentState: agentState,
		version:    version,
	}
	s.impl = newServiceImpl(s)
	return s
}

// SetRegisterFunc sets the callback invoked to register the device after Wi-Fi connects.
func (s *Service) SetRegisterFunc(fn RegisterFunc) {
	s.impl.setRegisterFunc(fn)
}

// Run starts the BLE service. It blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	s.impl.run(ctx)
}

// StartAdvertising sends a signal to begin a new advertising window.
func (s *Service) StartAdvertising() {
	s.impl.startAdvertising()
}

// OnClaimed transitions to provisioned state and stops advertising.
func (s *Service) OnClaimed() {
	s.impl.onClaimed()
}
