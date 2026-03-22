package sdr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFromSysfs(t *testing.T) {
	// Create a temporary sysfs-like directory structure.
	tmpDir := t.TempDir()

	// Create a valid RTL-SDR device.
	devDir := filepath.Join(tmpDir, "1-1")
	os.MkdirAll(devDir, 0755)
	os.WriteFile(filepath.Join(devDir, "idVendor"), []byte("0bda\n"), 0644)
	os.WriteFile(filepath.Join(devDir, "idProduct"), []byte("2838\n"), 0644)
	os.WriteFile(filepath.Join(devDir, "serial"), []byte("SKT-OMNI-0\n"), 0644)
	os.WriteFile(filepath.Join(devDir, "product"), []byte("RTL2838UHIDIR R820T\n"), 0644)
	os.WriteFile(filepath.Join(devDir, "busnum"), []byte("1\n"), 0644)
	os.WriteFile(filepath.Join(devDir, "devnum"), []byte("3\n"), 0644)

	// Create a non-RTL device (should be ignored).
	otherDir := filepath.Join(tmpDir, "1-2")
	os.MkdirAll(otherDir, 0755)
	os.WriteFile(filepath.Join(otherDir, "idVendor"), []byte("1234\n"), 0644)
	os.WriteFile(filepath.Join(otherDir, "idProduct"), []byte("5678\n"), 0644)

	devices := detectFromSysfs(tmpDir)

	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	dev := devices[0]
	if dev.VendorID != "0bda" {
		t.Errorf("vendor = %q, want 0bda", dev.VendorID)
	}
	if dev.ProductID != "2838" {
		t.Errorf("product = %q, want 2838", dev.ProductID)
	}
	if dev.SerialNumber != "SKT-OMNI-0" {
		t.Errorf("serial = %q, want SKT-OMNI-0", dev.SerialNumber)
	}
	if dev.TunerType != "R820T" {
		t.Errorf("tuner = %q, want R820T", dev.TunerType)
	}
	if dev.USBBusNum != 1 {
		t.Errorf("busnum = %d, want 1", dev.USBBusNum)
	}
	if dev.USBDevNum != 3 {
		t.Errorf("devnum = %d, want 3", dev.USBDevNum)
	}
}

func TestDeriveTunerType(t *testing.T) {
	tests := []struct {
		product string
		want    string
	}{
		{"RTL2838UHIDIR R820T", "R820T"},
		{"RTL2838 R820T2", "R820T2"},
		{"RTL2832U R828D", "R828D"},
		{"Generic USB Device", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		got := deriveTunerType(tt.product)
		if got != tt.want {
			t.Errorf("deriveTunerType(%q) = %q, want %q", tt.product, got, tt.want)
		}
	}
}

func TestFilterAvailable(t *testing.T) {
	devices := []SDRDevice{
		{SerialNumber: "ADSB-001", TunerType: "R820T"},
		{SerialNumber: "SKT-OMNI-0", TunerType: "R820T"},
		{SerialNumber: "SKT-OMNI-1", TunerType: "R828D"},
	}

	available := FilterAvailable(devices, "ADSB-001")
	if len(available) != 2 {
		t.Fatalf("expected 2 available devices, got %d", len(available))
	}
	if available[0].SerialNumber != "SKT-OMNI-0" {
		t.Errorf("first available = %q, want SKT-OMNI-0", available[0].SerialNumber)
	}
}

func TestDetermineMode(t *testing.T) {
	tests := []struct {
		readsbActive  bool
		availableSDRs int
		want          Mode
	}{
		{true, 2, ModeADSBOmni},
		{true, 0, ModeADSBOnly},
		{false, 1, ModeOmniOnly},
		{false, 0, ModeNone},
	}

	for _, tt := range tests {
		got := DetermineMode(tt.readsbActive, tt.availableSDRs)
		if got != tt.want {
			t.Errorf("DetermineMode(%v, %d) = %q, want %q", tt.readsbActive, tt.availableSDRs, got, tt.want)
		}
	}
}

func TestDetectEmptySysfs(t *testing.T) {
	tmpDir := t.TempDir()
	devices := detectFromSysfs(tmpDir)
	if len(devices) != 0 {
		t.Errorf("expected 0 devices from empty sysfs, got %d", len(devices))
	}
}

func TestDetectNonexistentSysfs(t *testing.T) {
	devices := detectFromSysfs("/nonexistent/path")
	if len(devices) != 0 {
		t.Errorf("expected 0 devices from nonexistent path, got %d", len(devices))
	}
}
