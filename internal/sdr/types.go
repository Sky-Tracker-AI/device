package sdr

// SDRDevice describes an RTL-SDR dongle discovered via sysfs.
type SDRDevice struct {
	SysfsPath    string
	USBBusNum    int
	USBDevNum    int
	VendorID     string
	ProductID    string
	SerialNumber string
	TunerType    string // "R820T", "R820T2", "R828D", "unknown"
}

// SDRHandle is the interface used by the scheduler to reference an SDR device.
type SDRHandle interface {
	ID() string
	SerialNumber() string
	TunerType() string
}

// Mode describes the operating mode determined by hardware detection.
type Mode string

const (
	ModeADSBOmni Mode = "adsb_omni"  // readsb + extra SDRs for Omni
	ModeADSBOnly Mode = "adsb_only"  // readsb active, no extra SDRs
	ModeOmniOnly Mode = "omni_only"  // no readsb, SDRs available
	ModeNone     Mode = "none"       // no readsb, no SDRs (prediction-only)
)

// deviceHandle is a concrete SDRHandle backed by an SDRDevice.
type deviceHandle struct {
	dev SDRDevice
}

// NewHandle creates an SDRHandle from an SDRDevice.
func NewHandle(dev SDRDevice) SDRHandle {
	return &deviceHandle{dev: dev}
}

func (h *deviceHandle) ID() string {
	if h.dev.SerialNumber != "" {
		return h.dev.SerialNumber
	}
	return h.dev.SysfsPath
}

func (h *deviceHandle) SerialNumber() string {
	return h.dev.SerialNumber
}

func (h *deviceHandle) TunerType() string {
	return h.dev.TunerType
}

// MockSDRHandle is a mock SDR handle for testing.
type MockSDRHandle struct {
	MockID     string
	MockSerial string
	MockTuner  string
}

func (m *MockSDRHandle) ID() string           { return m.MockID }
func (m *MockSDRHandle) SerialNumber() string  { return m.MockSerial }
func (m *MockSDRHandle) TunerType() string     { return m.MockTuner }
