package uat

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ClassifyFrame determines whether a raw JSON frame is an ADS-B aircraft frame,
// a FIS-B uplink frame, or something else.
func ClassifyFrame(frame RawFrame) string {
	line := frame.Line
	if strings.Contains(line, `"address"`) {
		// Check for adsb_ type prefix.
		if idx := strings.Index(line, `"type":"`); idx >= 0 {
			after := line[idx+len(`"type":"`):]
			if strings.HasPrefix(after, "adsb_") {
				return "adsb"
			}
		}
		// Has address but not adsb type — could be TIS-B trackfile (Phase 4).
		return "unknown"
	}
	if strings.Contains(line, `"tisb_site_id"`) && strings.Contains(line, `"products"`) {
		return "uplink"
	}
	return "unknown"
}

// uplinkFrame is the JSON structure of a dump978 uplink frame.
type uplinkFrame struct {
	TISBSiteID string          `json:"tisb_site_id"`
	Timestamp  float64         `json:"timestamp"`
	Products   []uplinkProduct `json:"products"`
}

type uplinkProduct struct {
	ProductID int    `json:"product_id"`
	Data      string `json:"data"`
}

// ParseUplinkProducts parses a FIS-B uplink frame and returns parsed products.
// Returns the parsed products, the site ID, and any error.
func ParseUplinkProducts(frame RawFrame) ([]FISBProduct, string, error) {
	var uf uplinkFrame
	if err := json.Unmarshal([]byte(frame.Line), &uf); err != nil {
		return nil, "", fmt.Errorf("unmarshal uplink frame: %w", err)
	}

	now := time.Now().UnixMilli()
	if uf.Timestamp > 0 {
		now = int64(uf.Timestamp * 1000)
	}

	var products []FISBProduct
	for _, p := range uf.Products {
		if p.Data == "" {
			continue
		}
		product, ok := routeByProductID(p.ProductID, p.Data, uf.TISBSiteID, now)
		if ok {
			products = append(products, *product)
		}
	}
	return products, uf.TISBSiteID, nil
}

// routeByProductID dispatches to the appropriate parser based on FIS-B product ID.
func routeByProductID(productID int, data string, siteID string, timestamp int64) (*FISBProduct, bool) {
	switch {
	case productID == 63 || productID == 64 || productID == 413:
		// NEXRAD radar — Phase 3, skip for now.
		return nil, false
	case productID >= 70 && productID <= 83:
		// TFR graphics — Phase 3, skip for now.
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
		Timestamp:   timestamp,
		ProductID:   16,
		ProductName: "pirep",
		RawText:     text,
		SiteID:      siteID,
		Severity:    severity,
		AltitudeLow: altLow,
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
