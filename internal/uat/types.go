package uat

// UATAircraft is a parsed ADS-B UAT position report from dump978.
type UATAircraft struct {
	Address          string  // ICAO hex address (24-bit)
	AddressQualifier string  // "adsb_icao", "adsb_icao_nt", "tisb_icao", "tisb_trackfile", "adsr_icao"
	Lat              float64
	Lon              float64
	AltBaro          int     // Barometric altitude in feet
	AltGeom          int     // Geometric altitude in feet
	GS               float64 // Ground speed in knots
	Track            float64 // Track/heading in degrees
	VertRate         int     // Vertical rate in ft/min
	Squawk           string  // Transponder squawk code
	Flight           string  // Callsign / flight ID (may have trailing spaces)
	Category         string  // Emitter category (A0-D7)
	NIC              int     // Navigation Integrity Category
	NACp             int     // Navigation Accuracy Category - Position
	Timestamp        float64 // Unix timestamp with fractional seconds
}

// RawFrame is a raw JSON line from dump978 stdout.
type RawFrame struct {
	Line string
}

// DecoderStats tracks running statistics for the UAT decoder.
type DecoderStats struct {
	FramesDecoded int
	FrameRate     float64 // Frames per minute (rolling)
	UptimeSeconds int64
	LastFrameAt   int64 // Unix milliseconds
	Running       bool
}
