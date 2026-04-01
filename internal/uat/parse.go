package uat

import (
	"encoding/json"
	"strings"
)

// ParseFrame parses a raw dump978 JSON frame into a UATAircraft.
// Returns false if the frame is not a valid ADS-B position report.
func ParseFrame(frame RawFrame) (UATAircraft, bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(frame.Line), &raw); err != nil {
		return UATAircraft{}, false
	}

	// Must have an address field.
	addr, ok := raw["address"].(string)
	if !ok || addr == "" {
		return UATAircraft{}, false
	}

	// Filter to ADS-B frames only (skip TIS-B for Phase 1).
	if t, ok := raw["type"].(string); ok {
		if !strings.HasPrefix(t, "adsb_") {
			return UATAircraft{}, false
		}
	}

	a := UATAircraft{
		Address: strings.ToUpper(addr),
	}

	if v, ok := raw["type"].(string); ok {
		a.AddressQualifier = v
	}
	if v, ok := raw["lat"].(float64); ok {
		a.Lat = v
	}
	if v, ok := raw["lon"].(float64); ok {
		a.Lon = v
	}
	if v, ok := raw["alt_baro"].(float64); ok {
		a.AltBaro = int(v)
	}
	if v, ok := raw["alt_geom"].(float64); ok {
		a.AltGeom = int(v)
	}
	if v, ok := raw["gs"].(float64); ok {
		a.GS = v
	}
	if v, ok := raw["track"].(float64); ok {
		a.Track = v
	}
	if v, ok := raw["vert_rate"].(float64); ok {
		a.VertRate = int(v)
	}
	if v, ok := raw["squawk"].(string); ok {
		a.Squawk = v
	}
	if v, ok := raw["flight"].(string); ok {
		a.Flight = v
	}
	if v, ok := raw["category"].(string); ok {
		a.Category = v
	}
	if v, ok := raw["nic"].(float64); ok {
		a.NIC = int(v)
	}
	if v, ok := raw["nacp"].(float64); ok {
		a.NACp = int(v)
	}
	if v, ok := raw["timestamp"].(float64); ok {
		a.Timestamp = v
	}

	// Must have a position to be useful.
	if a.Lat == 0 && a.Lon == 0 {
		return UATAircraft{}, false
	}

	return a, true
}
