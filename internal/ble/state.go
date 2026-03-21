package ble

// ProvisioningState represents the current BLE provisioning stage.
type ProvisioningState uint8

const (
	StateAdvertising  ProvisioningState = 0x00
	StateConnected    ProvisioningState = 0x01
	StateWiFiPending  ProvisioningState = 0x02
	StateWiFiConnected ProvisioningState = 0x03
	StateClaimPending ProvisioningState = 0x04
	StateProvisioned  ProvisioningState = 0x05
	StateError        ProvisioningState = 0x06
)

func (s ProvisioningState) String() string {
	switch s {
	case StateAdvertising:
		return "advertising"
	case StateConnected:
		return "connected"
	case StateWiFiPending:
		return "wifi_pending"
	case StateWiFiConnected:
		return "wifi_connected"
	case StateClaimPending:
		return "claim_pending"
	case StateProvisioned:
		return "provisioned"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}
