package acars

// ACARSMessage is a parsed, classified ACARS message ready for platform ingest.
// Fields match the platform's ACARSMessage model (PRD §5.3).
type ACARSMessage struct {
	Timestamp      int64   // Unix milliseconds
	Source         string  // "aero" or "stdc"
	MessageType    string  // pos, acars, wx, oooi, mil, sar, nav, egc
	AESHex         string  // Aircraft Earth Station hex address
	ICAOHex        string  // Mapped from AES if available
	Callsign       string  // Flight callsign (e.g. "UAL955")
	Registration   string  // Aircraft registration (e.g. "N77012")
	AircraftType   string  // ICAO type code (e.g. "B77W")
	Lat            float64 // Position latitude (pos/sar messages)
	Lon            float64 // Position longitude
	Altitude       int     // Altitude in feet (pos messages)
	Heading        float64 // Heading in degrees
	Speed          float64 // Ground speed in knots
	ETAAirport     string  // Destination ICAO code from pos reports
	ETATime        int64   // Estimated arrival time (Unix ms)
	Label          string  // ACARS label code (2-char)
	Sublabel       string  // ACARS sublabel
	RawText        string  // Raw message text (PII redacted)
	DecodedSummary string  // Human-readable decode
	Frequency      float64 // Receive frequency in MHz
	SignalStrength float64 // Signal strength in dB
	SatID          string  // Inmarsat satellite identifier (e.g. "4F3")
	Channel        string  // Inmarsat channel identifier
	OOOIEvent      string  // "out", "off", "on", "in" (OOOI only)
	OOOIAirport    string  // Airport ICAO for OOOI event
}

// ACARSRawMessage is a raw message line from SatDump stdout before parsing.
type ACARSRawMessage struct {
	Line string // Raw JSON line from SatDump
}

// DecoderStats tracks running statistics for the ACARS decoder.
type DecoderStats struct {
	MessagesDecoded int
	MessageRate     float64 // Messages per minute (rolling 5-minute window)
	PeakSNR         float64
	CurrentSNR      float64
	UptimeSeconds   int64
	LastMessageAt   int64 // Unix milliseconds
	Synced          bool  // Deframer lock status
}
