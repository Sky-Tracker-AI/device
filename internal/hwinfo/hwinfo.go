// Package hwinfo collects hardware metadata from the host system.
// Static info (board model, OS, CPU, RAM) is collected once at startup for
// registration. Dynamic info (CPU temp, disk usage) is collected each health tick.
package hwinfo

// StaticInfo contains hardware metadata that rarely changes.
type StaticInfo struct {
	BoardModel    string
	OSPrettyName  string
	KernelVersion string
	CPUModel      string
	TotalMemoryMB int
}

// DynamicInfo contains system metrics that change over time.
type DynamicInfo struct {
	CPUTempC    float64
	DiskFreeMB  int64
	DiskTotalMB int64
}
