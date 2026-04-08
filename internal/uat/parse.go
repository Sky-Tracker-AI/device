package uat

import (
	"encoding/json"
	"strings"
)

// ParseFrame parses a raw dump978-fa JSON frame into a UATAircraft.
// Returns false if the frame is not a valid ADS-B position report.
//
// dump978-fa JSON format (--json-port / --json-stdout):
//
//	{
//	  "address": "a714a1",
//	  "address_qualifier": "adsb_icao",
//	  "position": {"lat": 30.23671, "lon": -97.57784},
//	  "pressure_altitude": 1500,
//	  "geometric_altitude": 1750,
//	  "ground_speed": 87,
//	  "true_track": 258.8,
//	  "vertical_velocity_geometric": -128,
//	  "callsign": "N5552E",
//	  "emitter_category": "A1",
//	  "metadata": {"received_at": 1775666500.709, "rssi": -4.1}
//	}
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

	// Filter to ADS-B frames only (skip TIS-B for now).
	if q, ok := raw["address_qualifier"].(string); ok {
		if !strings.HasPrefix(q, "adsb_") {
			return UATAircraft{}, false
		}
	}

	a := UATAircraft{
		Address: strings.ToUpper(addr),
	}

	if v, ok := raw["address_qualifier"].(string); ok {
		a.AddressQualifier = v
	}

	// Position is nested: {"position": {"lat": ..., "lon": ...}}
	if pos, ok := raw["position"].(map[string]any); ok {
		if v, ok := pos["lat"].(float64); ok {
			a.Lat = v
		}
		if v, ok := pos["lon"].(float64); ok {
			a.Lon = v
		}
	}

	if v, ok := raw["pressure_altitude"].(float64); ok {
		a.AltBaro = int(v)
	}
	if v, ok := raw["geometric_altitude"].(float64); ok {
		a.AltGeom = int(v)
	}
	if v, ok := raw["ground_speed"].(float64); ok {
		a.GS = v
	}
	if v, ok := raw["true_track"].(float64); ok {
		a.Track = v
	}
	if v, ok := raw["vertical_velocity_geometric"].(float64); ok {
		a.VertRate = int(v)
	}
	if v, ok := raw["callsign"].(string); ok {
		a.Flight = v
	}
	if v, ok := raw["emitter_category"].(string); ok {
		a.Category = v
	}
	if v, ok := raw["nic"].(float64); ok {
		a.NIC = int(v)
	}
	if v, ok := raw["nac_p"].(float64); ok {
		a.NACp = int(v)
	}
	if meta, ok := raw["metadata"].(map[string]any); ok {
		if v, ok := meta["received_at"].(float64); ok {
			a.Timestamp = v
		}
	}
	if v, ok := raw["flightplan_id"].(string); ok {
		a.Squawk = v
	}

	// Must have a position to be useful.
	if a.Lat == 0 && a.Lon == 0 {
		return UATAircraft{}, false
	}

	return a, true
}
