package sat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skytracker/skytracker-device/internal/omni"
)

const testTLEData = `ISS (ZARYA)
1 25544U 98067A   24001.50000000  .00016717  00000-0  10270-3 0  9006
2 25544  51.6400 208.9163 0006703  40.5765 319.5613 15.49560532437500
NOAA 19
1 33591U 09005A   24001.50000000  .00000084  00000-0  66943-4 0  9994
2 33591  99.1936 325.4726 0014147 102.9528 257.3253 14.12501399778045
UNKNOWN SAT
1 99999U 24001A   24001.50000000  .00000000  00000-0  00000-0 0  9999
2 99999   0.0000   0.0000 0000001   0.0000   0.0000  1.00000000    01
`

func TestParseTLEText(t *testing.T) {
	entries, err := parseTLEText(strings.NewReader(testTLEData))
	if err != nil {
		t.Fatalf("parseTLEText: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	iss := entries[0]
	if iss.NoradID != 25544 {
		t.Errorf("ISS NORAD ID = %d, want 25544", iss.NoradID)
	}
	if iss.Name != "ISS (ZARYA)" {
		t.Errorf("ISS name = %q, want %q", iss.Name, "ISS (ZARYA)")
	}
	if !strings.HasPrefix(iss.Line1, "1 ") {
		t.Errorf("ISS line1 doesn't start with '1 '")
	}

	noaa := entries[1]
	if noaa.NoradID != 33591 {
		t.Errorf("NOAA NORAD ID = %d, want 33591", noaa.NoradID)
	}
}

func TestParseTLETextInvalid(t *testing.T) {
	// Lines that don't match TLE format should be skipped.
	data := "SATELLITE NAME\nNot a TLE line 1\nNot a TLE line 2\n"
	entries, err := parseTLEText(strings.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries from invalid data, got %d", len(entries))
	}
}

func TestFetcherWithHTTPTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testTLEData))
	}))
	defer server.Close()

	catalog := map[int]*omni.CatalogEntry{
		25544: {NoradID: 25544, Name: "ISS (ZARYA)", Category: omni.CatSpaceStation, Decodable: true},
		33591: {NoradID: 33591, Name: "NOAA 19", Category: omni.CatWeather, Decodable: true},
	}

	cachePath := filepath.Join(t.TempDir(), "tles.json")
	fetcher := NewFetcher(catalog, cachePath, 12)

	// Fetch from our test server.
	entries, err := fetcher.fetchGroupTLE(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetchGroupTLE: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 TLE entries, got %d", len(entries))
	}
}

func TestFetcherDiskCache(t *testing.T) {
	catalog := map[int]*omni.CatalogEntry{
		25544: {NoradID: 25544, Name: "ISS (ZARYA)", Category: omni.CatSpaceStation, Decodable: true},
		33591: {NoradID: 33591, Name: "NOAA 19", Category: omni.CatWeather, Decodable: true},
	}

	cachePath := filepath.Join(t.TempDir(), "tles.json")

	// Create first fetcher and manually add TLEs.
	f1 := NewFetcher(catalog, cachePath, 12)
	f1.mu.Lock()
	f1.tles[25544] = &TLESet{
		NoradID:   25544,
		Name:      "ISS (ZARYA)",
		Line1:     "1 25544U 98067A   24001.50000000  .00016717  00000-0  10270-3 0  9006",
		Line2:     "2 25544  51.6400 208.9163 0006703  40.5765 319.5613 15.49560532437500",
	}
	f1.mu.Unlock()

	// Save to disk.
	if err := f1.saveToDisk(); err != nil {
		t.Fatalf("saveToDisk: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}

	// Create second fetcher and load from disk.
	f2 := NewFetcher(catalog, cachePath, 12)
	loaded, _ := f2.loadFromDisk()
	if loaded != 1 {
		t.Fatalf("expected 1 loaded from disk, got %d", loaded)
	}

	tle := f2.GetTLE(25544)
	if tle == nil {
		t.Fatal("ISS TLE not loaded from disk")
	}
	if tle.Name != "ISS (ZARYA)" {
		t.Errorf("ISS name = %q, want %q", tle.Name, "ISS (ZARYA)")
	}
}

func TestFetcherCount(t *testing.T) {
	catalog := omni.BuildCatalogIndex()
	cachePath := filepath.Join(t.TempDir(), "tles.json")
	f := NewFetcher(catalog, cachePath, 12)

	if f.Count() != 0 {
		t.Errorf("expected 0 count initially, got %d", f.Count())
	}
}
