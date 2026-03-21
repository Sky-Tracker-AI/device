package adsb

import "encoding/json"

// FlexInt handles JSON fields that can be either a number or a string
// (e.g. dump1090's alt_baro which is an int in flight but "ground" on surface).
// String values unmarshal as nil.
type FlexInt struct {
	Value *int
}

func (f *FlexInt) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		f.Value = &n
		return nil
	}
	// Not a number (e.g. "ground") — leave Value as nil so callers
	// can distinguish "on ground" from "no data."
	f.Value = nil
	return nil
}

func (f FlexInt) MarshalJSON() ([]byte, error) {
	if f.Value == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(*f.Value)
}

// Aircraft represents a single aircraft as reported by dump1090-fa.
type Aircraft struct {
	Hex         string   `json:"hex"`
	Flight      string   `json:"flight,omitempty"`
	AltBaro     *FlexInt `json:"alt_baro,omitempty"`
	AltGeom     *int     `json:"alt_geom,omitempty"`
	GS          *float64 `json:"gs,omitempty"`
	Track       *float64 `json:"track,omitempty"`
	Lat         *float64 `json:"lat,omitempty"`
	Lon         *float64 `json:"lon,omitempty"`
	Category    string   `json:"category,omitempty"`
	BaroRate    *int     `json:"baro_rate,omitempty"`
	Squawk      string   `json:"squawk,omitempty"`
	Emergency   string   `json:"emergency,omitempty"`
	RSSI        float64  `json:"rssi,omitempty"`
	Seen        float64  `json:"seen,omitempty"`
	SeenPos     float64  `json:"seen_pos,omitempty"`
	Messages    int      `json:"messages,omitempty"`
}

// Dump1090Response is the top-level JSON response from dump1090-fa
// at /data/aircraft.json.
type Dump1090Response struct {
	Now      float64    `json:"now"`
	Messages int        `json:"messages"`
	Aircraft []Aircraft `json:"aircraft"`
}

// Altitude returns the best available altitude in feet. Prefers barometric
// altitude, falls back to geometric.
func (a *Aircraft) Altitude() int {
	if a.AltBaro != nil && a.AltBaro.Value != nil {
		return *a.AltBaro.Value
	}
	if a.AltGeom != nil {
		return *a.AltGeom
	}
	return 0
}

// Speed returns ground speed in knots, or 0 if unavailable.
func (a *Aircraft) Speed() float64 {
	if a.GS != nil {
		return *a.GS
	}
	return 0
}

// Heading returns the track in degrees, or 0 if unavailable.
func (a *Aircraft) Heading() float64 {
	if a.Track != nil {
		return *a.Track
	}
	return 0
}

// VertRate returns barometric rate of change in ft/min, or 0 if unavailable.
func (a *Aircraft) VertRate() int {
	if a.BaroRate != nil {
		return *a.BaroRate
	}
	return 0
}

// HasPosition returns true if the aircraft has a valid lat/lon.
func (a *Aircraft) HasPosition() bool {
	return a.Lat != nil && a.Lon != nil
}

// Callsign returns the trimmed flight/callsign string.
func (a *Aircraft) Callsign() string {
	// dump1090 pads callsigns with spaces
	cs := a.Flight
	for len(cs) > 0 && cs[len(cs)-1] == ' ' {
		cs = cs[:len(cs)-1]
	}
	return cs
}
