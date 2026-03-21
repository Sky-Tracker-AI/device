//go:build linux

package ble

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/skytracker/skytracker-device/internal/wifi"
	"tinygo.org/x/bluetooth"
)

// UUIDs matching the iOS BLEConstants.swift definitions.
var (
	serviceUUID = bluetooth.NewUUID([16]byte{
		0x41, 0x47, 0xA5, 0xE0, 0xCA, 0xFE, 0x4A, 0x3E,
		0x9C, 0x37, 0x5A, 0x4B, 0x8D, 0x32, 0xFC, 0x00,
	})

	charWiFiSSID = bluetooth.NewUUID([16]byte{
		0x41, 0x47, 0xA5, 0xE0, 0xCA, 0xFE, 0x4A, 0x3E,
		0x9C, 0x37, 0x5A, 0x4B, 0x8D, 0x32, 0xFC, 0x01,
	})
	charWiFiPSK = bluetooth.NewUUID([16]byte{
		0x41, 0x47, 0xA5, 0xE0, 0xCA, 0xFE, 0x4A, 0x3E,
		0x9C, 0x37, 0x5A, 0x4B, 0x8D, 0x32, 0xFC, 0x02,
	})
	charClaimToken = bluetooth.NewUUID([16]byte{
		0x41, 0x47, 0xA5, 0xE0, 0xCA, 0xFE, 0x4A, 0x3E,
		0x9C, 0x37, 0x5A, 0x4B, 0x8D, 0x32, 0xFC, 0x03,
	})
	charProvState = bluetooth.NewUUID([16]byte{
		0x41, 0x47, 0xA5, 0xE0, 0xCA, 0xFE, 0x4A, 0x3E,
		0x9C, 0x37, 0x5A, 0x4B, 0x8D, 0x32, 0xFC, 0x04,
	})
	charProvMessage = bluetooth.NewUUID([16]byte{
		0x41, 0x47, 0xA5, 0xE0, 0xCA, 0xFE, 0x4A, 0x3E,
		0x9C, 0x37, 0x5A, 0x4B, 0x8D, 0x32, 0xFC, 0x05,
	})
	charDeviceInfo = bluetooth.NewUUID([16]byte{
		0x41, 0x47, 0xA5, 0xE0, 0xCA, 0xFE, 0x4A, 0x3E,
		0x9C, 0x37, 0x5A, 0x4B, 0x8D, 0x32, 0xFC, 0x06,
	})
	charWiFiResults = bluetooth.NewUUID([16]byte{
		0x41, 0x47, 0xA5, 0xE0, 0xCA, 0xFE, 0x4A, 0x3E,
		0x9C, 0x37, 0x5A, 0x4B, 0x8D, 0x32, 0xFC, 0x07,
	})
	charCommand = bluetooth.NewUUID([16]byte{
		0x41, 0x47, 0xA5, 0xE0, 0xCA, 0xFE, 0x4A, 0x3E,
		0x9C, 0x37, 0x5A, 0x4B, 0x8D, 0x32, 0xFC, 0x08,
	})
)

type serviceImpl struct {
	mu sync.Mutex
	s  *Service

	state     ProvisioningState
	message   string
	connCount int
	connected bool

	// BLE handles for notifications.
	stateChar       bluetooth.Characteristic
	messageChar     bluetooth.Characteristic
	claimTokenChar  bluetooth.Characteristic
	wifiResultsChar bluetooth.Characteristic

	// Pending Wi-Fi credentials.
	pendingSSID string
	pendingPSK  string

	// Signal channels.
	startAdv chan struct{}

	// Registration callback for first-boot scenario.
	registerFunc RegisterFunc

	adapter *bluetooth.Adapter
}

func newServiceImpl(s *Service) serviceImpl {
	return serviceImpl{
		s:        s,
		state:    StateAdvertising,
		startAdv: make(chan struct{}, 1),
		adapter:  bluetooth.DefaultAdapter,
	}
}

func (si *serviceImpl) setRegisterFunc(fn RegisterFunc) {
	si.mu.Lock()
	defer si.mu.Unlock()
	si.registerFunc = fn
}

func (si *serviceImpl) run(ctx context.Context) {
	if err := si.adapter.Enable(); err != nil {
		log.Printf("[ble] failed to enable adapter: %v", err)
		return
	}

	if err := si.addGATTService(); err != nil {
		log.Printf("[ble] failed to add GATT service: %v", err)
		return
	}

	log.Printf("[ble] GATT service registered, waiting for advertising trigger")

	for {
		select {
		case <-ctx.Done():
			log.Printf("[ble] shutting down")
			return
		case <-si.startAdv:
			si.advertise(ctx)
		}
	}
}

func (si *serviceImpl) startAdvertising() {
	select {
	case si.startAdv <- struct{}{}:
	default:
	}
}

func (si *serviceImpl) onClaimed() {
	si.setState(StateProvisioned, "Device claimed successfully")
}

func (si *serviceImpl) addGATTService() error {
	return si.adapter.AddService(&bluetooth.Service{
		UUID: serviceUUID,
		Characteristics: []bluetooth.CharacteristicConfig{
			// FC01: Wi-Fi SSID (Write)
			{
				UUID:  charWiFiSSID,
				Flags: bluetooth.CharacteristicWritePermission | bluetooth.CharacteristicWriteWithoutResponsePermission,
				WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
					si.handleWiFiSSID(value)
				},
			},
			// FC02: Wi-Fi PSK (Write)
			{
				UUID:  charWiFiPSK,
				Flags: bluetooth.CharacteristicWritePermission | bluetooth.CharacteristicWriteWithoutResponsePermission,
				WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
					si.handleWiFiPSK(value)
				},
			},
			// FC03: Claim Token (Read, Notify)
			{
				Handle: &si.claimTokenChar,
				UUID:   charClaimToken,
				Flags:  bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicNotifyPermission,
				Value:  []byte(si.s.agentState.GetClaimCode()),
			},
			// FC04: Provisioning State (Read, Notify)
			{
				Handle: &si.stateChar,
				UUID:   charProvState,
				Flags:  bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicNotifyPermission,
				Value:  []byte{byte(si.state)},
			},
			// FC05: Provisioning Message (Read, Notify)
			{
				Handle: &si.messageChar,
				UUID:   charProvMessage,
				Flags:  bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicNotifyPermission,
				Value:  []byte(si.message),
			},
			// FC06: Device Info (Read)
			{
				UUID:  charDeviceInfo,
				Flags: bluetooth.CharacteristicReadPermission,
				Value: si.deviceInfoJSON(),
			},
			// FC07: Wi-Fi Scan Results (Read, Notify)
			{
				Handle: &si.wifiResultsChar,
				UUID:   charWiFiResults,
				Flags:  bluetooth.CharacteristicReadPermission | bluetooth.CharacteristicNotifyPermission,
				Value:  []byte("[]"),
			},
			// FC08: Command (Write)
			{
				UUID:  charCommand,
				Flags: bluetooth.CharacteristicWritePermission | bluetooth.CharacteristicWriteWithoutResponsePermission,
				WriteEvent: func(client bluetooth.Connection, offset int, value []byte) {
					si.handleCommand(value)
				},
			},
		},
	})
}

func (si *serviceImpl) advertise(ctx context.Context) {
	si.mu.Lock()
	si.state = StateAdvertising
	si.message = "Waiting for connection..."
	si.connCount = 0
	si.connected = false
	si.pendingSSID = ""
	si.pendingPSK = ""
	si.mu.Unlock()

	// Build device name: prefix + last 6 chars of serial.
	serial := si.s.agentState.GetSerial()
	suffix := serial
	if len(suffix) > 6 {
		suffix = suffix[len(suffix)-6:]
	}
	deviceName := si.s.cfg.DeviceNamePrefix + suffix

	adv := si.adapter.DefaultAdvertisement()
	if err := adv.Configure(bluetooth.AdvertisementOptions{
		LocalName:    deviceName,
		ServiceUUIDs: []bluetooth.UUID{serviceUUID},
	}); err != nil {
		log.Printf("[ble] failed to configure advertisement: %v", err)
		return
	}

	if err := adv.Start(); err != nil {
		log.Printf("[ble] failed to start advertising: %v", err)
		return
	}

	log.Printf("[ble] advertising as %q for %ds", deviceName, si.s.cfg.WindowSeconds)

	si.notifyState()

	timer := time.NewTimer(time.Duration(si.s.cfg.WindowSeconds) * time.Second)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		adv.Stop()
		log.Printf("[ble] advertising stopped (context cancelled)")
	case <-timer.C:
		adv.Stop()
		log.Printf("[ble] advertising window expired")
	}
}

func (si *serviceImpl) setState(newState ProvisioningState, msg string) {
	si.mu.Lock()
	si.state = newState
	si.message = msg
	si.mu.Unlock()

	log.Printf("[ble] state → %s: %s", newState, msg)
	si.notifyState()
}

func (si *serviceImpl) notifyState() {
	si.mu.Lock()
	st := si.state
	msg := si.message
	si.mu.Unlock()

	si.stateChar.Write([]byte{byte(st)})
	si.messageChar.Write([]byte(msg))
}

func (si *serviceImpl) handleWiFiSSID(value []byte) {
	si.mu.Lock()
	si.pendingSSID = string(value)
	ssid := si.pendingSSID
	psk := si.pendingPSK
	si.mu.Unlock()

	log.Printf("[ble] received Wi-Fi SSID: %s", ssid)

	if ssid != "" && psk != "" {
		si.connectWiFi(ssid, psk)
	}
}

func (si *serviceImpl) handleWiFiPSK(value []byte) {
	si.mu.Lock()
	si.pendingPSK = string(value)
	ssid := si.pendingSSID
	psk := si.pendingPSK
	si.mu.Unlock()

	log.Printf("[ble] received Wi-Fi PSK (len=%d)", len(psk))

	if ssid != "" && psk != "" {
		si.connectWiFi(ssid, psk)
	}
}

func (si *serviceImpl) connectWiFi(ssid, psk string) {
	si.setState(StateWiFiPending, "Connecting to Wi-Fi...")

	go func() {
		err := si.s.wifiMgr.Connect(ssid, psk)
		if err != nil {
			switch {
			case err == wifi.ErrAuthFailed:
				si.setState(StateError, "Wi-Fi authentication failed")
			case err == wifi.ErrTimeout:
				si.setState(StateError, "Wi-Fi connection timed out")
			default:
				si.setState(StateError, fmt.Sprintf("Wi-Fi error: %v", err))
			}
			si.mu.Lock()
			si.pendingSSID = ""
			si.pendingPSK = ""
			si.mu.Unlock()
			return
		}

		si.setState(StateWiFiConnected, "Wi-Fi connected")

		si.mu.Lock()
		si.pendingSSID = ""
		si.pendingPSK = ""
		regFn := si.registerFunc
		si.mu.Unlock()

		// If device is not yet registered, attempt registration.
		if !si.s.agentState.IsRegistered() && regFn != nil {
			si.setState(StateClaimPending, "Registering device...")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := regFn(ctx); err != nil {
				log.Printf("[ble] registration after wifi failed: %v", err)
			}
		}

		// Update claim token characteristic.
		if claimCode := si.s.agentState.GetClaimCode(); claimCode != "" {
			si.claimTokenChar.Write([]byte(claimCode))
		}

		// If already claimed, go straight to provisioned.
		if si.s.agentState.GetClaimed() {
			si.setState(StateProvisioned, "Device already claimed")
		}
	}()
}

func (si *serviceImpl) handleCommand(value []byte) {
	if len(value) == 0 {
		return
	}

	switch value[0] {
	case 0x01: // SCAN_WIFI
		log.Printf("[ble] command: scan WiFi")
		go func() {
			networks, err := si.s.wifiMgr.ScanNetworks()
			if err != nil {
				log.Printf("[ble] WiFi scan failed: %v", err)
				si.wifiResultsChar.Write([]byte("[]"))
				return
			}
			data, err := json.Marshal(networks)
			if err != nil {
				log.Printf("[ble] marshal scan results: %v", err)
				return
			}
			si.wifiResultsChar.Write(data)
			log.Printf("[ble] sent %d WiFi networks", len(networks))
		}()

	case 0x02: // FACTORY_RESET
		log.Printf("[ble] command: factory reset (deferred to V2)")

	default:
		log.Printf("[ble] unknown command: 0x%02x", value[0])
	}
}

func (si *serviceImpl) deviceInfoJSON() []byte {
	info := struct {
		Serial    string `json:"serial"`
		FWVersion string `json:"fw_version"`
	}{
		Serial:    si.s.agentState.GetSerial(),
		FWVersion: si.s.version,
	}
	data, _ := json.Marshal(info)
	return data
}
