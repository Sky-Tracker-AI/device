//go:build linux

package hwinfo

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// CollectStatic gathers hardware metadata from Linux pseudo-files.
func CollectStatic() StaticInfo {
	return StaticInfo{
		BoardModel:    readBoardModel(),
		OSPrettyName:  readOSPrettyName(),
		KernelVersion: readKernelVersion(),
		CPUModel:      readCPUModel(),
		TotalMemoryMB: readTotalMemoryMB(),
	}
}

// CollectDynamic gathers runtime system metrics.
func CollectDynamic() DynamicInfo {
	freeMB, totalMB := readDiskUsage("/var/lib/skytracker")
	return DynamicInfo{
		CPUTempC:    readCPUTemp(),
		DiskFreeMB:  freeMB,
		DiskTotalMB: totalMB,
	}
}

func readBoardModel() string {
	data, err := os.ReadFile("/proc/device-tree/model")
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(data), "\x00\n ")
}

func readOSPrettyName() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if val, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(val, "\"")
		}
	}
	return ""
}

func readKernelVersion() string {
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err != nil {
		return ""
	}
	b := make([]byte, 0, len(uname.Release))
	for _, v := range uname.Release {
		if v == 0 {
			break
		}
		b = append(b, byte(v))
	}
	return string(b)
}

func readCPUModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// x86: "model name : ..."
		// ARM: "Model        : ..."
		if strings.HasPrefix(line, "model name") || strings.HasPrefix(line, "Model") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func readTotalMemoryMB() int {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.Atoi(fields[1])
				if err != nil {
					return 0
				}
				return kb / 1024
			}
		}
	}
	return 0
}

func readCPUTemp() float64 {
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0
	}
	millideg, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return float64(millideg) / 1000.0
}

func readDiskUsage(path string) (freeMB, totalMB int64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	freeMB = int64(stat.Bavail) * int64(stat.Bsize) / (1024 * 1024)
	totalMB = int64(stat.Blocks) * int64(stat.Bsize) / (1024 * 1024)
	return freeMB, totalMB
}
