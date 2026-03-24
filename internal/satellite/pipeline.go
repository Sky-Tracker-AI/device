package satellite

// PipelineConfig maps a satellite to its SatDump decoding parameters.
type PipelineConfig struct {
	PipelineID string // "noaa_apt", "meteor_m2-x_lrpt"
	SampleRate int    // Hz
	Protocol   string // "APT", "LRPT"
	SatNumber  int    // for NOAA APT: 15, 18, or 19
}

// pipelineMap holds known satellite NORAD ID → SatDump pipeline configurations.
var pipelineMap = map[int]*PipelineConfig{
	// NOAA APT satellites
	25338: {PipelineID: "noaa_apt", SampleRate: 1024000, Protocol: "APT", SatNumber: 15},
	28654: {PipelineID: "noaa_apt", SampleRate: 1024000, Protocol: "APT", SatNumber: 18},
	33591: {PipelineID: "noaa_apt", SampleRate: 1024000, Protocol: "APT", SatNumber: 19},

	// METEOR LRPT satellites
	57166: {PipelineID: "meteor_m2-x_lrpt", SampleRate: 1024000, Protocol: "LRPT"},
	44387: {PipelineID: "meteor_m2-x_lrpt", SampleRate: 1024000, Protocol: "LRPT"},
	40069: {PipelineID: "meteor_m2-x_lrpt", SampleRate: 1024000, Protocol: "LRPT"},

}

// GetPipeline returns the SatDump pipeline config for a satellite, or nil if unknown.
func GetPipeline(noradID int) *PipelineConfig {
	return pipelineMap[noradID]
}
