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

// LatLon is a geographic coordinate pair.
type LatLon struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// FISBProduct is a parsed FIS-B weather product from a UAT uplink frame.
type FISBProduct struct {
	Timestamp      int64    `json:"timestamp"`
	ProductID      int      `json:"product_id"`
	ProductName    string   `json:"product_name"`
	ReportID       string   `json:"report_id,omitempty"`
	AirportICAO    string   `json:"airport_icao,omitempty"`
	RawText        string   `json:"raw_text"`
	Lat            float64  `json:"lat,omitempty"`
	Lon            float64  `json:"lon,omitempty"`
	GeoPolygon     []LatLon `json:"geo_polygon,omitempty"`
	AltitudeLow    int      `json:"altitude_low,omitempty"`
	AltitudeHigh   int      `json:"altitude_high,omitempty"`
	ValidFrom      int64    `json:"valid_from,omitempty"`
	ValidUntil     int64    `json:"valid_until,omitempty"`
	SiteID         string   `json:"site_id,omitempty"`
	Severity       string   `json:"severity,omitempty"`
	FlightCategory string   `json:"flight_category,omitempty"`
}

// DecoderStats tracks running statistics for the UAT decoder.
type DecoderStats struct {
	FramesDecoded       int
	FrameRate           float64 // Frames per minute (rolling)
	UptimeSeconds       int64
	LastFrameAt         int64 // Unix milliseconds
	Running             bool
	FISBProductsDecoded int     `json:"fisb_products_decoded"`
	FISBProductRate     float64 `json:"fisb_product_rate"`
	FISBLastProductAt   int64   `json:"fisb_last_product_at"`
}
