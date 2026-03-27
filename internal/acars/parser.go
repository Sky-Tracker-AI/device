package acars

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
)

// Parser processes raw ACARS messages from the decoder, classifying
// and enriching them before handing off to the platform sync goroutine.
type Parser struct {
	aesDB  *AESDatabase
	input  <-chan ACARSRawMessage
	output chan ACARSMessage
}

// NewParser creates a parser that reads raw messages and writes parsed messages.
func NewParser(input <-chan ACARSRawMessage, aesDB *AESDatabase) *Parser {
	return &Parser{
		aesDB:  aesDB,
		input:  input,
		output: make(chan ACARSMessage, 500),
	}
}

// Run reads from the input channel, parses, and writes to the output channel.
// Blocks until ctx is cancelled or the input channel is closed.
func (p *Parser) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case raw, ok := <-p.input:
			if !ok {
				return
			}
			msg, err := p.parseRawMessage(raw)
			if err != nil {
				log.Printf("[acars] parse error: %v", err)
				continue
			}
			msg.MessageType = classifyMessage(msg)
			msg.DecodedSummary = generateDecodedSummary(msg)
			msg.RawText = redactPII(msg.RawText)
			p.enrichWithAES(msg)

			select {
			case p.output <- *msg:
			default:
				log.Printf("[acars] parser output channel full, dropping message")
			}
		}
	}
}

// Parsed returns the output channel of parsed messages.
func (p *Parser) Parsed() <-chan ACARSMessage {
	return p.output
}

// parseRawMessage extracts fields from a SatDump JSON line into an ACARSMessage.
func (p *Parser) parseRawMessage(raw ACARSRawMessage) (*ACARSMessage, error) {
	if raw.Line == "" {
		return nil, fmt.Errorf("empty message line")
	}

	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(raw.Line), &fields); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	msg := &ACARSMessage{
		Timestamp: time.Now().UnixMilli(),
		Source:    "aero",
		RawText:   raw.Line,
	}

	// Extract known SatDump Inmarsat Aero fields.
	if v, ok := fields["aes_id"].(string); ok {
		msg.AESHex = v
	}
	if v, ok := fields["registration"].(string); ok {
		msg.Registration = v
	}
	if v, ok := fields["callsign"].(string); ok {
		msg.Callsign = v
	}
	if v, ok := fields["flight"].(string); ok && msg.Callsign == "" {
		msg.Callsign = v
	}
	if v, ok := fields["label"].(string); ok {
		msg.Label = v
	}
	if v, ok := fields["sublabel"].(string); ok {
		msg.Sublabel = v
	}
	if v, ok := fields["text"].(string); ok {
		msg.RawText = v
	}
	if v, ok := fields["message"].(string); ok && msg.RawText == raw.Line {
		msg.RawText = v
	}
	if v, ok := fields["aircraft_type"].(string); ok {
		msg.AircraftType = v
	}

	// Position fields.
	if v, ok := fields["lat"].(float64); ok {
		msg.Lat = v
	}
	if v, ok := fields["lon"].(float64); ok {
		msg.Lon = v
	}
	if v, ok := fields["altitude"].(float64); ok {
		msg.Altitude = int(v)
	}
	if v, ok := fields["heading"].(float64); ok {
		msg.Heading = v
	}
	if v, ok := fields["speed"].(float64); ok {
		msg.Speed = v
	}

	// ETA fields.
	if v, ok := fields["eta_airport"].(string); ok {
		msg.ETAAirport = v
	}
	if v, ok := fields["eta_time"].(float64); ok {
		msg.ETATime = int64(v)
	}

	// Signal metadata.
	if v, ok := fields["frequency"].(float64); ok {
		msg.Frequency = v
	}
	if v, ok := fields["signal_strength"].(float64); ok {
		msg.SignalStrength = v
	}
	if v, ok := fields["sat_id"].(string); ok {
		msg.SatID = v
	}
	if v, ok := fields["channel"].(string); ok {
		msg.Channel = v
	}

	// Source (aero vs stdc).
	if v, ok := fields["source"].(string); ok {
		msg.Source = v
	}

	// OOOI fields.
	if v, ok := fields["oooi_event"].(string); ok {
		msg.OOOIEvent = v
	}
	if v, ok := fields["oooi_airport"].(string); ok {
		msg.OOOIAirport = v
	}

	return msg, nil
}

// classifyMessage determines the message type based on content patterns.
func classifyMessage(msg *ACARSMessage) string {
	text := strings.ToUpper(msg.RawText)

	// Position report: contains lat/lon/alt/hdg/spd.
	if msg.Lat != 0 && msg.Lon != 0 && msg.Altitude > 0 && msg.Heading > 0 && msg.Speed > 0 {
		return "pos"
	}
	if strings.Contains(text, "LAT") && strings.Contains(text, "LON") &&
		strings.Contains(text, "ALT") && strings.Contains(text, "HDG") &&
		strings.Contains(text, "SPD") {
		return "pos"
	}

	// Weather delivery.
	if strings.Contains(text, "METAR") || strings.Contains(text, "TAF ") || strings.Contains(text, "SIGMET") {
		return "wx"
	}

	// OOOI events.
	if msg.OOOIEvent != "" {
		return "oooi"
	}
	if msg.Label == "20" || msg.Label == "10" {
		// ACARS labels 10/20 are common OOOI label codes.
		for _, kw := range []string{"OUT ", "OFF ", "ON ", "IN ", "PUSHBACK", "WHEELS UP", "WHEELS DOWN", "GATE "} {
			if strings.Contains(text, kw) {
				return "oooi"
			}
		}
	}

	// Military dispatch (callsign prefixes).
	if msg.Callsign != "" {
		cs := strings.ToUpper(msg.Callsign)
		for _, prefix := range []string{"RCH", "EVAC", "REACH", "KING", "DUKE", "JAKE", "TEAL", "ORDER", "GORDO", "TOPCT"} {
			if strings.HasPrefix(cs, prefix) {
				return "mil"
			}
		}
	}

	// STD-C / EGC messages.
	if msg.Source == "stdc" {
		if strings.Contains(text, "SAR") || strings.Contains(text, "DISTRESS") ||
			strings.Contains(text, "MAYDAY") || strings.Contains(text, "SEARCH AND RESCUE") {
			return "sar"
		}
		if strings.Contains(text, "NAVAREA") || strings.Contains(text, "HAZARD") ||
			strings.Contains(text, "UNLIT") || strings.Contains(text, "ADRIFT") {
			return "nav"
		}
		return "egc"
	}

	// SAR with coordinates in Aero messages.
	if strings.Contains(text, "USCG") || strings.Contains(text, "COAST GUARD") {
		if msg.Lat != 0 && msg.Lon != 0 {
			return "sar"
		}
	}

	return "acars"
}

// generateDecodedSummary produces a human-readable summary of the message.
func generateDecodedSummary(msg *ACARSMessage) string {
	switch msg.MessageType {
	case "pos":
		parts := []string{
			fmt.Sprintf("Position: %.2f°%s, %.2f°%s",
				absFloat(msg.Lat), latDir(msg.Lat),
				absFloat(msg.Lon), lonDir(msg.Lon)),
		}
		if msg.Altitude > 0 {
			parts = append(parts, fmt.Sprintf("FL%d", msg.Altitude/100))
		}
		if msg.Heading > 0 {
			parts = append(parts, fmt.Sprintf("heading %.0f°", msg.Heading))
		}
		if msg.Speed > 0 {
			parts = append(parts, fmt.Sprintf("%.0fkt", msg.Speed))
		}
		summary := strings.Join(parts, ", ")
		if msg.ETAAirport != "" {
			summary += fmt.Sprintf(". ETA %s", msg.ETAAirport)
			if msg.ETATime > 0 {
				t := time.UnixMilli(msg.ETATime).UTC()
				summary += fmt.Sprintf(" at %s", t.Format("15:04"))
			}
		}
		return summary + "."

	case "wx":
		if msg.Callsign != "" {
			return fmt.Sprintf("Weather delivery for %s.", msg.Callsign)
		}
		return "Weather report delivery."

	case "oooi":
		event := strings.ToUpper(msg.OOOIEvent)
		eventName := map[string]string{
			"OUT": "Pushback from gate",
			"OFF": "Wheels up (takeoff)",
			"ON":  "Wheels down (landing)",
			"IN":  "Arrived at gate",
		}[event]
		if eventName == "" {
			eventName = "Flight event"
		}
		if msg.OOOIAirport != "" {
			return fmt.Sprintf("%s at %s.", eventName, msg.OOOIAirport)
		}
		return eventName + "."

	case "mil":
		if msg.Callsign != "" {
			return fmt.Sprintf("Military dispatch: %s.", msg.Callsign)
		}
		return "Military dispatch message."

	case "sar":
		summary := "Search and rescue alert"
		if msg.Lat != 0 && msg.Lon != 0 {
			summary += fmt.Sprintf(" at %.2f°%s, %.2f°%s",
				absFloat(msg.Lat), latDir(msg.Lat),
				absFloat(msg.Lon), lonDir(msg.Lon))
		}
		return summary + "."

	case "nav":
		return "Navigation warning."

	case "egc":
		return "Maritime safety broadcast (EGC)."

	default:
		if msg.Callsign != "" {
			return fmt.Sprintf("ACARS message from %s.", msg.Callsign)
		}
		return "ACARS message."
	}
}

// PII patterns for redaction.
var piiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)CAPT\s+[A-Z][A-Z'-]+`),           // Captain names
	regexp.MustCompile(`(?i)F/?O\s+[A-Z][A-Z'-]+`),            // First officer names
	regexp.MustCompile(`(?i)CREW\s*:?\s*[A-Z][A-Z'-]+`),       // Crew names
	regexp.MustCompile(`(?i)PAX\s+NAME\s*:?\s*[A-Z][A-Z'-]+`), // Passenger names
	regexp.MustCompile(`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`),       // Phone numbers
	regexp.MustCompile(`\b[A-Z0-9]{6}\b(?:\s+NAME)`),          // PNR with NAME context
}

// redactPII strips personally identifiable information from message text.
func redactPII(text string) string {
	for _, pat := range piiPatterns {
		text = pat.ReplaceAllString(text, "[REDACTED]")
	}
	return text
}

// enrichWithAES populates ICAO hex, registration, and type from the AES database.
func (p *Parser) enrichWithAES(msg *ACARSMessage) {
	if msg.AESHex == "" || p.aesDB == nil {
		return
	}
	entry := p.aesDB.Lookup(msg.AESHex)
	if entry == nil {
		return
	}
	if msg.ICAOHex == "" {
		msg.ICAOHex = entry.ICAOHex
	}
	if msg.Registration == "" {
		msg.Registration = entry.Registration
	}
	if msg.AircraftType == "" {
		msg.AircraftType = entry.TypeCode
	}
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func latDir(lat float64) string {
	if lat >= 0 {
		return "N"
	}
	return "S"
}

func lonDir(lon float64) string {
	if lon >= 0 {
		return "E"
	}
	return "W"
}
