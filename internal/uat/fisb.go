package uat

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// uplinkFrameDataBytes is the size of a decoded UAT uplink frame (432 bytes = 864 hex chars).
	uplinkFrameDataBytes = 432

	// dlacAlpha is the DLAC (Data Link Alphabet Character) lookup table.
	// Index 0 = ETX (end of text), index 28 = tab (expands to spaces).
	dlacAlpha = "\x03ABCDEFGHIJKLMNOPQRSTUVWXYZ\x1A\t\x1E\n| !\"#$%&'()*+,-./0123456789:;<=>?"
)

// ClassifyFrame determines whether a raw JSON frame from the json-port is an
// ADS-B aircraft frame or something else. Uplink/FIS-B frames are NOT emitted
// on the JSON port — they arrive via the raw port and are handled separately.
func ClassifyFrame(frame RawFrame) string {
	line := frame.Line
	if strings.Contains(line, `"address"`) {
		// dump978-fa uses "address_qualifier":"adsb_icao" (not "type").
		if strings.Contains(line, `"address_qualifier":"adsb_`) {
			return "adsb"
		}
		// Has address but not adsb qualifier — could be TIS-B.
		return "unknown"
	}
	return "unknown"
}

// DecodeRawUplink decodes a raw hex uplink line from dump978-fa's --raw-port
// into FIS-B weather products. The line format is: +<864 hex chars>;ss=N;rs=N
func DecodeRawUplink(line string) ([]FISBProduct, error) {
	// Strip metadata after semicolon.
	parts := strings.SplitN(line, ";", 2)
	hexStr := parts[0]

	if len(hexStr) < 1 || hexStr[0] != '+' {
		return nil, fmt.Errorf("not an uplink frame")
	}
	hexStr = hexStr[1:] // strip '+' prefix

	if len(hexStr) != uplinkFrameDataBytes*2 {
		return nil, fmt.Errorf("wrong uplink length: %d hex chars (expected %d)", len(hexStr), uplinkFrameDataBytes*2)
	}

	frame := make([]byte, uplinkFrameDataBytes)
	if _, err := hex.Decode(frame, []byte(hexStr)); err != nil {
		return nil, fmt.Errorf("hex decode: %w", err)
	}

	// Extract TIS-B site ID (byte 7, upper nibble).
	siteID := fmt.Sprintf("%d", frame[7]>>4)

	// Check app_data_valid flag (byte 6, bit 5).
	if frame[6]&0x20 == 0 {
		return nil, nil // no application data
	}

	now := time.Now().UnixMilli()

	// Parse info frames from application data (bytes 8–431).
	appData := frame[8:uplinkFrameDataBytes]
	var products []FISBProduct
	pos := 0
	maxFrames := len(appData) / 6 // upper bound on info frames

	for i := 0; i < maxFrames && pos+2 <= len(appData); i++ {
		frameLen := int((uint32(appData[pos]) << 1) | (uint32(appData[pos+1]) >> 7))
		frameType := uint32(appData[pos+1]) & 0x0f

		if frameLen == 0 {
			break
		}
		if pos+2+frameLen > len(appData) {
			break
		}

		infoData := appData[pos+2 : pos+2+frameLen]
		pos += 2 + frameLen

		// Only process FIS-B APDUs (frame_type 0).
		if frameType != 0 || len(infoData) < 3 {
			continue
		}

		// Extract product ID (11 bits spanning bytes 0-1).
		productID := int(((uint32(infoData[0]) & 0x1f) << 6) | (uint32(infoData[1]) >> 2))

		// Decode DLAC text from the FIS-B payload.
		text := decodeFISBText(infoData, uint32(frameLen))
		if text == "" {
			continue
		}

		// Split multi-report frames on record separators.
		reports := splitReports(text)
		for _, report := range reports {
			report = strings.TrimSpace(report)
			if report == "" {
				continue
			}
			product, ok := routeByProductID(productID, report, siteID, now)
			if ok {
				products = append(products, *product)
			}
		}
	}

	return products, nil
}

// decodeFISBText extracts and DLAC-decodes the text payload from a FIS-B info
// frame. The frame starts with 2 bytes of product ID/flags, followed by a
// variable-length time header (2–4 additional bytes), then DLAC-encoded text.
func decodeFISBText(data []byte, frameLen uint32) string {
	if len(data) < 3 {
		return ""
	}

	// t_opt is 2 bits: data[1] bit 0 and data[2] bit 7.
	tOpt := ((uint32(data[1]) & 0x01) << 1) | (uint32(data[2]) >> 7)

	var headerLen uint32
	switch tOpt {
	case 0:
		headerLen = 4 // hours, minutes
	case 1:
		headerLen = 5 // hours, minutes, seconds
	case 2:
		headerLen = 5 // month, day, hours, minutes
	case 3:
		headerLen = 6 // month, day, hours, minutes, seconds
	default:
		return ""
	}

	if frameLen < headerLen {
		return ""
	}
	if uint32(len(data)) < headerLen {
		return ""
	}

	fisbData := data[headerLen:]
	fisbLen := frameLen - headerLen
	if uint32(len(fisbData)) < fisbLen {
		return ""
	}

	return dlacDecode(fisbData, fisbLen)
}

// dlacDecode unpacks 6-bit DLAC characters from a byte stream.
// Every 3 bytes encode 4 characters (24 bits → 4 × 6-bit chars).
func dlacDecode(data []byte, dataLen uint32) string {
	step := 0
	tab := false
	var sb strings.Builder

	for i := uint32(0); i < dataLen; i++ {
		var ch uint32
		switch step {
		case 0:
			ch = uint32(data[i]) >> 2
		case 1:
			ch = ((uint32(data[i-1]) & 0x03) << 4) | (uint32(data[i]) >> 4)
		case 2:
			ch = ((uint32(data[i-1]) & 0x0f) << 2) | (uint32(data[i]) >> 6)
			i-- // byte shared between two characters
		case 3:
			ch = uint32(data[i]) & 0x3f
		}

		if tab {
			for ch > 0 {
				sb.WriteByte(' ')
				ch--
			}
			tab = false
		} else if ch == 28 {
			tab = true
		} else if int(ch) < len(dlacAlpha) {
			sb.WriteByte(dlacAlpha[ch])
		}

		step = (step + 1) % 4
	}

	return sb.String()
}

// splitReports splits DLAC-decoded text into individual reports.
// Reports are separated by \x1E (record separator) or \x03 (end of text).
func splitReports(text string) []string {
	// Replace ETX with record separator so we can split once.
	text = strings.ReplaceAll(text, "\x03", "\x1E")
	parts := strings.Split(text, "\x1E")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// routeByProductID dispatches to the appropriate parser based on FIS-B product ID.
func routeByProductID(productID int, data string, siteID string, timestamp int64) (*FISBProduct, bool) {
	switch {
	case productID == 63 || productID == 64:
		// NEXRAD radar imagery — graphical product, skip.
		return nil, false
	case productID >= 70 && productID <= 83:
		// TFR graphics — graphical product, skip.
		return nil, false
	case productID == 14:
		return parseMETAR(data, siteID, timestamp), true
	case productID == 15:
		return parseTAF(data, siteID, timestamp), true
	case productID == 8:
		return parseNOTAM(data, siteID, timestamp), true
	case productID == 16:
		return parsePIREP(data, siteID, timestamp), true
	case productID == 17:
		return parseWindsAloft(data, siteID, timestamp), true
	case productID == 11:
		return parseTextProduct("airmet", productID, data, siteID, timestamp), true
	case productID == 12:
		return parseTextProduct("sigmet", productID, data, siteID, timestamp), true
	case productID == 13:
		return parseTextProduct("sua", productID, data, siteID, timestamp), true
	default:
		return parseTextProduct("unknown", productID, data, siteID, timestamp), true
	}
}

// icaoRe matches a 4-letter ICAO airport code (e.g. KAUS, KJFK).
var icaoRe = regexp.MustCompile(`\b[A-Z]{4}\b`)

// visibilityRe matches visibility values before "SM" (e.g. "10SM", "3/4SM", "1 1/2SM").
var visibilityRe = regexp.MustCompile(`(\d+\s+)?(\d+/\d+|\d+)SM`)

// ceilingRe matches cloud layers with BKN or OVC (e.g. "BKN045", "OVC010").
var ceilingRe = regexp.MustCompile(`(BKN|OVC)(\d{3})`)

// metarTimeRe matches the observation time group (e.g. "251453Z").
var metarTimeRe = regexp.MustCompile(`(\d{6})Z`)

// parseMETAR parses a METAR/SPECI text product.
func parseMETAR(data string, siteID string, timestamp int64) *FISBProduct {
	text := strings.TrimSpace(data)

	// Extract airport ICAO: first 4-letter code after METAR/SPECI keyword.
	airport := ""
	stripped := text
	for _, prefix := range []string{"METAR ", "SPECI "} {
		if idx := strings.Index(stripped, prefix); idx >= 0 {
			stripped = stripped[idx+len(prefix):]
			break
		}
	}
	if m := icaoRe.FindString(stripped); m != "" {
		airport = m
	}

	// Build report ID from airport and observation time.
	reportID := ""
	if airport != "" {
		if m := metarTimeRe.FindStringSubmatch(text); len(m) >= 2 {
			reportID = airport + "-" + m[1]
		}
	}

	// Compute flight category.
	category := computeFlightCategory(text)

	return &FISBProduct{
		Timestamp:      timestamp,
		ProductID:      14,
		ProductName:    "metar",
		ReportID:       reportID,
		AirportICAO:    airport,
		RawText:        text,
		SiteID:         siteID,
		FlightCategory: category,
	}
}

// computeFlightCategory determines VFR/MVFR/IFR/LIFR from METAR text.
func computeFlightCategory(text string) string {
	vis := parseVisibility(text)
	ceiling := parseCeiling(text)

	// LIFR: ceiling < 500 OR visibility < 1
	if ceiling >= 0 && ceiling < 500 || vis >= 0 && vis < 1 {
		return "LIFR"
	}
	// IFR: ceiling 500-999 OR visibility 1-2.99
	if ceiling >= 500 && ceiling < 1000 || vis >= 1 && vis < 3 {
		return "IFR"
	}
	// MVFR: ceiling 1000-3000 OR visibility 3-5
	if ceiling >= 1000 && ceiling <= 3000 || vis >= 3 && vis <= 5 {
		return "MVFR"
	}
	// VFR: ceiling > 3000 AND visibility > 5 (or no restrictions)
	return "VFR"
}

// parseVisibility extracts visibility in statute miles from METAR text.
// Returns -1 if not found.
func parseVisibility(text string) float64 {
	m := visibilityRe.FindStringSubmatch(text)
	if len(m) == 0 {
		return -1
	}

	whole := strings.TrimSpace(m[1])
	frac := m[2]

	var vis float64

	// Parse whole number part.
	if whole != "" {
		w, err := strconv.ParseFloat(whole, 64)
		if err == nil {
			vis += w
		}
	}

	// Parse fractional or integer part.
	if strings.Contains(frac, "/") {
		parts := strings.SplitN(frac, "/", 2)
		num, err1 := strconv.ParseFloat(parts[0], 64)
		den, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 == nil && err2 == nil && den > 0 {
			vis += num / den
		}
	} else {
		v, err := strconv.ParseFloat(frac, 64)
		if err == nil {
			vis += v
		}
	}

	return vis
}

// parseCeiling finds the lowest BKN or OVC layer altitude in feet.
// Returns -1 if no ceiling layers found.
func parseCeiling(text string) int {
	matches := ceilingRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return -1
	}

	lowest := -1
	for _, m := range matches {
		hundreds, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		alt := hundreds * 100
		if lowest < 0 || alt < lowest {
			lowest = alt
		}
	}
	return lowest
}

// parseTAF parses a TAF forecast text product.
func parseTAF(data string, siteID string, timestamp int64) *FISBProduct {
	text := strings.TrimSpace(data)

	// Extract airport ICAO after "TAF".
	airport := ""
	if idx := strings.Index(text, "TAF"); idx >= 0 {
		after := text[idx+3:]
		if m := icaoRe.FindString(after); m != "" {
			airport = m
		}
	}

	// Parse valid period (e.g. "2512/2612") for report ID.
	reportID := ""
	if airport != "" {
		validRe := regexp.MustCompile(`(\d{4}/\d{4})`)
		if m := validRe.FindString(text); m != "" {
			reportID = airport + "-TAF-" + m
		}
	}

	return &FISBProduct{
		Timestamp:   timestamp,
		ProductID:   15,
		ProductName: "taf",
		ReportID:    reportID,
		AirportICAO: airport,
		RawText:     text,
		SiteID:      siteID,
	}
}

// notamNumberRe matches NOTAM numbers like "1/2345" or "FDC 1/2345".
var notamNumberRe = regexp.MustCompile(`(\d+/\d+)`)

// parseNOTAM parses a NOTAM or TFR text product.
func parseNOTAM(data string, siteID string, timestamp int64) *FISBProduct {
	text := strings.TrimSpace(data)

	productName := "notam"
	if strings.HasPrefix(text, "!FDC") {
		upper := strings.ToUpper(text)
		if strings.Contains(upper, "TEMPORARY FLIGHT RESTRICTIONS") ||
			strings.Contains(upper, "TEMPORARY FLIGHT RESTRICTION") {
			productName = "tfr"
		}
	}

	// Extract NOTAM number for report ID.
	reportID := ""
	if m := notamNumberRe.FindString(text); m != "" {
		reportID = "NOTAM-" + m
	}

	return &FISBProduct{
		Timestamp:   timestamp,
		ProductID:   8,
		ProductName: productName,
		ReportID:    reportID,
		RawText:     text,
		SiteID:      siteID,
	}
}

// pirepSeverityRe matches turbulence (/TB) or icing (/IC) severity fields.
var pirepSeverityRe = regexp.MustCompile(`/(TB|IC)\s+(NEG|LGT|MOD|SEV|EXTRM)`)

// pirepFLRe matches flight level (/FL) fields.
var pirepFLRe = regexp.MustCompile(`/FL(\d+)`)

// parsePIREP parses a pilot report text product.
func parsePIREP(data string, siteID string, timestamp int64) *FISBProduct {
	text := strings.TrimSpace(data)

	// Extract severity from /TB or /IC fields.
	severity := ""
	if m := pirepSeverityRe.FindStringSubmatch(text); len(m) >= 3 {
		severity = m[2]
	}

	// Extract flight level from /FL field.
	altLow := 0
	altHigh := 0
	if m := pirepFLRe.FindStringSubmatch(text); len(m) >= 2 {
		fl, err := strconv.Atoi(m[1])
		if err == nil {
			altLow = fl * 100
			altHigh = fl * 100
		}
	}

	return &FISBProduct{
		Timestamp:    timestamp,
		ProductID:    16,
		ProductName:  "pirep",
		RawText:      text,
		SiteID:       siteID,
		Severity:     severity,
		AltitudeLow:  altLow,
		AltitudeHigh: altHigh,
	}
}

// parseWindsAloft parses a winds aloft forecast product.
func parseWindsAloft(data string, siteID string, timestamp int64) *FISBProduct {
	text := strings.TrimSpace(data)

	return &FISBProduct{
		Timestamp:   timestamp,
		ProductID:   17,
		ProductName: "winds_aloft",
		RawText:     text,
		SiteID:      siteID,
	}
}

// parseTextProduct is a generic fallback for AIRMET, SIGMET, SUA, CWA, and
// unknown text products.
func parseTextProduct(name string, productID int, data string, siteID string, timestamp int64) *FISBProduct {
	text := strings.TrimSpace(data)

	return &FISBProduct{
		Timestamp:   timestamp,
		ProductID:   productID,
		ProductName: name,
		RawText:     text,
		SiteID:      siteID,
	}
}
