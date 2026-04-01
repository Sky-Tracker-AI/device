package sdr

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// RTL-SDR USB vendor/product IDs.
const (
	rtlVendorID  = "0bda"
	rtlProduct2832 = "2832"
	rtlProduct2838 = "2838"
)

// Detect enumerates RTL-SDR devices by walking /sys/bus/usb/devices/.
func Detect() []SDRDevice {
	return detectFromSysfs("/sys/bus/usb/devices")
}

func detectFromSysfs(sysfsBase string) []SDRDevice {
	entries, err := os.ReadDir(sysfsBase)
	if err != nil {
		log.Printf("[sdr] cannot read sysfs %s: %v", sysfsBase, err)
		return nil
	}

	var devices []SDRDevice
	for _, entry := range entries {
		devPath := filepath.Join(sysfsBase, entry.Name())
		vendor := readSysfsFile(filepath.Join(devPath, "idVendor"))
		if vendor != rtlVendorID {
			continue
		}

		product := readSysfsFile(filepath.Join(devPath, "idProduct"))
		if product != rtlProduct2832 && product != rtlProduct2838 {
			continue
		}

		dev := SDRDevice{
			SysfsPath:    devPath,
			VendorID:     vendor,
			ProductID:    product,
			SerialNumber: readSysfsFile(filepath.Join(devPath, "serial")),
			TunerType:    deriveTunerType(readSysfsFile(filepath.Join(devPath, "product"))),
		}

		dev.USBBusNum, _ = strconv.Atoi(readSysfsFile(filepath.Join(devPath, "busnum")))
		dev.USBDevNum, _ = strconv.Atoi(readSysfsFile(filepath.Join(devPath, "devnum")))

		devices = append(devices, dev)
	}

	return devices
}

// deriveTunerType guesses the tuner type from the USB product string.
func deriveTunerType(product string) string {
	p := strings.ToUpper(product)
	switch {
	case strings.Contains(p, "R828D"):
		return "R828D"
	case strings.Contains(p, "R820T2"):
		return "R820T2"
	case strings.Contains(p, "R820T"):
		return "R820T"
	default:
		return "unknown"
	}
}

// readSysfsFile reads a single-line sysfs attribute file.
func readSysfsFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// DetectReadsbSerial returns the serial number (or device index) that readsb
// is currently using, and whether readsb is active.
func DetectReadsbSerial() (serial string, active bool) {
	// Check if readsb is running.
	out, err := exec.Command("systemctl", "is-active", "readsb").Output()
	if err != nil || strings.TrimSpace(string(out)) != "active" {
		return "", false
	}

	// Parse /etc/default/readsb for --device flag.
	data, err := os.ReadFile("/etc/default/readsb")
	if err != nil {
		return "0", true
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "--device"); idx >= 0 {
			parts := strings.Fields(line[idx:])
			if len(parts) >= 2 {
				val := parts[1]
				if strings.Contains(parts[0], "=") {
					val = strings.SplitN(parts[0], "=", 2)[1]
				}
				return val, true
			}
		}
	}

	return "0", true
}

// FilterAvailable returns SDR devices not claimed by readsb.
func FilterAvailable(devices []SDRDevice, readsbSerial string) []SDRDevice {
	var available []SDRDevice
	for i, dev := range devices {
		if dev.SerialNumber != "" && dev.SerialNumber == readsbSerial {
			continue
		}
		// If readsb uses index "0" (no serial known), skip the first device by position.
		if readsbSerial == "0" && i == 0 {
			continue
		}
		available = append(available, dev)
	}
	return available
}

// DetermineMode determines the operating mode based on readsb status and available SDRs.
func DetermineMode(readsbActive bool, availableSDRs int) Mode {
	switch {
	case readsbActive && availableSDRs > 0:
		return ModeADSBOmni
	case readsbActive && availableSDRs == 0:
		return ModeADSBOnly
	case !readsbActive && availableSDRs > 0:
		return ModeOmniOnly
	default:
		return ModeNone
	}
}

// ReserveACARSSDR picks one SDR from the available pool for dedicated ACARS use.
// Prefers R828D tuners (best L-band performance), then R820T2, then R820T.
// Returns the reserved device and the remaining pool for the scheduler.
func ReserveACARSSDR(available []SDRDevice) (reserved *SDRDevice, remaining []SDRDevice) {
	if len(available) == 0 {
		return nil, nil
	}

	bestIdx := -1
	bestScore := -1
	for i, dev := range available {
		score := 0
		switch dev.TunerType {
		case "R828D":
			score = 3
		case "R820T2":
			score = 2
		case "R820T":
			score = 1
		default:
			score = 1
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx < 0 {
		return nil, available
	}

	dev := available[bestIdx]
	remaining = make([]SDRDevice, 0, len(available)-1)
	remaining = append(remaining, available[:bestIdx]...)
	remaining = append(remaining, available[bestIdx+1:]...)
	return &dev, remaining
}

// ReserveGOESSDR picks one SDR from the available pool for dedicated GOES HRIT use.
// Same tuner preference as ACARS (R828D > R820T2 > R820T for L-band performance).
// Returns the reserved device and the remaining pool for the scheduler.
func ReserveGOESSDR(available []SDRDevice) (reserved *SDRDevice, remaining []SDRDevice) {
	// Identical selection logic to ReserveACARSSDR — both are L-band SDRs.
	return ReserveACARSSDR(available)
}

// ReserveUATSDR picks one SDR from the available pool for dedicated 978 MHz UAT use.
// Prefers R820T2/R820T tuners (best VHF/UHF performance), then R828D.
// Returns the reserved device and the remaining pool.
func ReserveUATSDR(available []SDRDevice) (reserved *SDRDevice, remaining []SDRDevice) {
	if len(available) == 0 {
		return nil, nil
	}

	bestIdx := -1
	bestScore := -1
	for i, dev := range available {
		score := 0
		switch dev.TunerType {
		case "R820T2":
			score = 3
		case "R820T":
			score = 2
		case "R828D":
			score = 1
		default:
			score = 1
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx < 0 {
		return nil, available
	}

	dev := available[bestIdx]
	remaining = make([]SDRDevice, 0, len(available)-1)
	remaining = append(remaining, available[:bestIdx]...)
	remaining = append(remaining, available[bestIdx+1:]...)
	return &dev, remaining
}

// ProgramSerial programs a serial number onto an RTL-SDR dongle via rtl_eeprom.
// This is a one-time operation that persists in the dongle's EEPROM.
func ProgramSerial(deviceIndex int, serial string) error {
	rtlEeprom, err := exec.LookPath("rtl_eeprom")
	if err != nil {
		log.Printf("[sdr] rtl_eeprom not found, skipping serial programming: %v", err)
		return nil
	}

	cmd := exec.Command(rtlEeprom, "-d", strconv.Itoa(deviceIndex), "-s", serial)
	// rtl_eeprom prompts "Write new configuration to device [y/n]?" on stdin.
	cmd.Stdin = strings.NewReader("y\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rtl_eeprom failed: %w: %s", err, string(out))
	}
	log.Printf("[sdr] programmed serial %q on device %d", serial, deviceIndex)
	return nil
}

// ProgramSerials assigns SKT-OMNI-N serial numbers to SDR devices with
// empty or generic serials. Returns the number of devices programmed.
// Note: EEPROM changes only take effect after a USB replug, so the in-memory
// SerialNumber is set to the device index (e.g. "0") for the current session
// to ensure rtl_tcp can address the device.
func ProgramSerials(devices []SDRDevice) int {
	programmed := 0
	for i, dev := range devices {
		if dev.SerialNumber != "" && !strings.HasPrefix(dev.SerialNumber, "00000") {
			continue
		}
		serial := fmt.Sprintf("SKT-OMNI-%d", i)
		if err := ProgramSerial(i, serial); err != nil {
			log.Printf("[sdr] failed to program serial for device %d: %v", i, err)
			continue
		}
		// Use device index for the current session since the EEPROM serial
		// won't be visible to rtl_tcp until the dongle is physically replugged.
		devices[i].SerialNumber = strconv.Itoa(i)
		programmed++
	}
	return programmed
}

