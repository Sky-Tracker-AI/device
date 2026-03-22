package sat

import (
	"time"

	"github.com/skytracker/skytracker-device/internal/omni"
)

// TLESet holds a parsed TLE for a single satellite.
type TLESet struct {
	NoradID   int       `json:"norad_id"`
	Name      string    `json:"name"`
	Line1     string    `json:"line1"`
	Line2     string    `json:"line2"`
	Epoch     time.Time `json:"epoch"`
	FetchedAt time.Time `json:"fetched_at"`
}

// GroundStation describes an observer position on Earth.
type GroundStation struct {
	Lat  float64 `json:"lat"`   // degrees
	Lon  float64 `json:"lon"`   // degrees
	AltM float64 `json:"alt_m"` // meters above sea level
}

// PassPrediction describes a future pass of a satellite over a ground station.
type PassPrediction struct {
	NoradID      int              `json:"norad_id"`
	Name         string           `json:"name"`
	Category     omni.SatCategory `json:"category"`
	AOS          time.Time        `json:"aos"`
	LOS          time.Time        `json:"los"`
	MaxElevation float64          `json:"max_elevation"`
	MaxElevTime  time.Time        `json:"max_elev_time"`
	Decodable    bool             `json:"decodable"`
	Frequencies  []float64        `json:"frequencies"` // MHz, for SDR tuning
}
