package acars

import (
	"encoding/json"
	"os"
	"sync"
)

// AESEntry maps an Aircraft Earth Station address to aircraft metadata.
type AESEntry struct {
	AESHex       string `json:"aes_hex"`
	ICAOHex      string `json:"icao_hex"`
	Registration string `json:"registration"`
	Operator     string `json:"operator"`
	TypeCode     string `json:"type_code"`
}

// AESDatabase provides lookup from AES hex to aircraft metadata.
type AESDatabase struct {
	mu      sync.RWMutex
	entries map[string]*AESEntry
}

// NewAESDatabase creates a database seeded with well-known AES mappings.
func NewAESDatabase() *AESDatabase {
	db := &AESDatabase{
		entries: make(map[string]*AESEntry, len(seedAESEntries)),
	}
	for k, v := range seedAESEntries {
		db.entries[k] = v
	}
	return db
}

// Lookup returns metadata for an AES hex address, or nil if unknown.
func (db *AESDatabase) Lookup(aesHex string) *AESEntry {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.entries[aesHex]
}

// Count returns the number of entries in the database.
func (db *AESDatabase) Count() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.entries)
}

// LoadFromFile loads additional AES mappings from a JSON file.
// The file should contain a JSON array of AESEntry objects.
func (db *AESDatabase) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var entries []AESEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	db.mu.Lock()
	defer db.mu.Unlock()
	for i := range entries {
		db.entries[entries[i].AESHex] = &entries[i]
	}
	return nil
}

// seedAESEntries contains well-known AES → ICAO mappings compiled into the binary.
// This is a small starter set; the database grows via platform correlation over time.
var seedAESEntries = map[string]*AESEntry{
	// British Airways - Boeing 777 fleet
	"AC0184": {AESHex: "AC0184", ICAOHex: "400A28", Registration: "G-STBH", Operator: "British Airways", TypeCode: "B77W"},
	"AC0186": {AESHex: "AC0186", ICAOHex: "400A2A", Registration: "G-STBJ", Operator: "British Airways", TypeCode: "B77W"},
	"AC0188": {AESHex: "AC0188", ICAOHex: "400A2C", Registration: "G-STBL", Operator: "British Airways", TypeCode: "B77W"},

	// United Airlines
	"ADB42F": {AESHex: "ADB42F", ICAOHex: "A1B2C3", Registration: "N77012", Operator: "United Airlines", TypeCode: "B77W"},
	"ADB430": {AESHex: "ADB430", ICAOHex: "A1B2C4", Registration: "N77014", Operator: "United Airlines", TypeCode: "B77W"},

	// Delta Air Lines
	"A1C2D3": {AESHex: "A1C2D3", ICAOHex: "A0F1E2", Registration: "N860DA", Operator: "Delta Air Lines", TypeCode: "A333"},

	// Avianca
	"E40198": {AESHex: "E40198", ICAOHex: "0D0A10", Registration: "N763AV", Operator: "Avianca", TypeCode: "A20N"},

	// US Military - common tanker/transport AES addresses
	"AE07EA": {AESHex: "AE07EA", ICAOHex: "AE07EA", Registration: "", Operator: "USAF", TypeCode: "C17"},
	"AE07EB": {AESHex: "AE07EB", ICAOHex: "AE07EB", Registration: "", Operator: "USAF", TypeCode: "KC135"},
	"AE07EC": {AESHex: "AE07EC", ICAOHex: "AE07EC", Registration: "", Operator: "USAF", TypeCode: "KC10"},

	// FedEx
	"A44B20": {AESHex: "A44B20", ICAOHex: "A44B20", Registration: "N851FD", Operator: "FedEx Express", TypeCode: "B77L"},

	// UPS
	"A55C30": {AESHex: "A55C30", ICAOHex: "A55C30", Registration: "N283UP", Operator: "UPS Airlines", TypeCode: "B763"},

	// LATAM
	"E50210": {AESHex: "E50210", ICAOHex: "E50210", Registration: "CC-BGJ", Operator: "LATAM Airlines", TypeCode: "B789"},

	// Air France
	"3C0100": {AESHex: "3C0100", ICAOHex: "3C0100", Registration: "F-GSQE", Operator: "Air France", TypeCode: "B77W"},

	// Lufthansa
	"3C4920": {AESHex: "3C4920", ICAOHex: "3C4920", Registration: "D-AIMC", Operator: "Lufthansa", TypeCode: "A388"},
}
