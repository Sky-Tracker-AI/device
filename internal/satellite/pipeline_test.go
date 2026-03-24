package satellite

import "testing"

func TestGetPipeline(t *testing.T) {
	tests := []struct {
		name       string
		noradID    int
		wantNil    bool
		pipelineID string
		protocol   string
		satNumber  int
	}{
		{"NOAA 15", 25338, false, "noaa_apt", "APT", 15},
		{"NOAA 18", 28654, false, "noaa_apt", "APT", 18},
		{"NOAA 19", 33591, false, "noaa_apt", "APT", 19},
		{"METEOR-M N2-3", 57166, false, "meteor_m2-x_lrpt", "LRPT", 0},
		{"METEOR-M N2-2", 44387, false, "meteor_m2-x_lrpt", "LRPT", 0},
		{"METEOR-M N2", 40069, false, "meteor_m2-x_lrpt", "LRPT", 0},
		{"unknown satellite", 99999, true, "", "", 0},
		{"ISS (no pipeline)", 25544, true, "", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GetPipeline(tt.noradID)
			if tt.wantNil {
				if cfg != nil {
					t.Errorf("GetPipeline(%d) = %+v, want nil", tt.noradID, cfg)
				}
				return
			}
			if cfg == nil {
				t.Fatalf("GetPipeline(%d) = nil, want non-nil", tt.noradID)
			}
			if cfg.PipelineID != tt.pipelineID {
				t.Errorf("PipelineID = %q, want %q", cfg.PipelineID, tt.pipelineID)
			}
			if cfg.Protocol != tt.protocol {
				t.Errorf("Protocol = %q, want %q", cfg.Protocol, tt.protocol)
			}
			if cfg.SatNumber != tt.satNumber {
				t.Errorf("SatNumber = %d, want %d", cfg.SatNumber, tt.satNumber)
			}
			if cfg.SampleRate != 1024000 {
				t.Errorf("SampleRate = %d, want 1024000", cfg.SampleRate)
			}
		})
	}
}

func TestGetPipelineAllHaveSampleRate(t *testing.T) {
	for noradID, cfg := range pipelineMap {
		if cfg.SampleRate == 0 {
			t.Errorf("NORAD %d has zero SampleRate", noradID)
		}
		if cfg.PipelineID == "" {
			t.Errorf("NORAD %d has empty PipelineID", noradID)
		}
	}
}
