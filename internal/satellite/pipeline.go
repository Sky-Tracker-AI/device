package satellite

// PipelineConfig maps a satellite to its SatDump decoding parameters.
type PipelineConfig struct {
	PipelineID string // "meteor_m2-x_lrpt"
	SampleRate int    // Hz
	Protocol   string // "LRPT"
}

// pipelineMap holds known satellite NORAD ID → SatDump pipeline configurations.
// As of 2025, only METEOR-M LRPT satellites remain active on 137 MHz.
// NOAA 15/18/19 APT were decommissioned Jun-Aug 2025.
// METEOR-M N2 LRPT died Dec 2022; N2-2 LRPT transmitter permanently failed.
var pipelineMap = map[int]*PipelineConfig{
	57166: {PipelineID: "meteor_m2-x_lrpt", SampleRate: 1024000, Protocol: "LRPT"}, // METEOR-M N2-3
	59051: {PipelineID: "meteor_m2-x_lrpt", SampleRate: 1024000, Protocol: "LRPT"}, // METEOR-M N2-4
}

// GetPipeline returns the SatDump pipeline config for a satellite, or nil if unknown.
func GetPipeline(noradID int) *PipelineConfig {
	return pipelineMap[noradID]
}
