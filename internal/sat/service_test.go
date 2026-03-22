package sat

import (
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/skytracker/skytracker-device/internal/omni"
)

func TestServicePassesSorted(t *testing.T) {
	catalog := map[int]*omni.CatalogEntry{
		25544: {NoradID: 25544, Name: "ISS (ZARYA)", Category: omni.CatSpaceStation, Frequencies: []float64{145.8}, Decodable: true, IconSize: "large"},
		33591: {NoradID: 33591, Name: "NOAA 19", Category: omni.CatWeather, Frequencies: []float64{137.1}, Decodable: true, IconSize: "large"},
	}

	cachePath := filepath.Join(t.TempDir(), "tles.json")
	svc := &Service{
		fetcher:  NewFetcher(catalog, cachePath, 12),
		catalog:  catalog,
		minElev:  5.0,
		station:  GroundStation{Lat: 39.8561, Lon: -104.6737, AltM: 1609},
	}

	// Manually populate TLEs.
	svc.fetcher.mu.Lock()
	svc.fetcher.tles[25544] = &TLESet{
		NoradID:   25544,
		Name:      "ISS (ZARYA)",
		Line1:     "1 25544U 98067A   24001.50000000  .00016717  00000-0  10270-3 0  9006",
		Line2:     "2 25544  51.6400 208.9163 0006703  40.5765 319.5613 15.49560532437500",
		FetchedAt: time.Now(),
	}
	svc.fetcher.tles[33591] = &TLESet{
		NoradID:   33591,
		Name:      "NOAA 19",
		Line1:     "1 33591U 09005A   24001.50000000  .00000084  00000-0  66943-4 0  9994",
		Line2:     "2 33591  99.1936 325.4726 0014147 102.9528 257.3253 14.12501399778045",
		FetchedAt: time.Now(),
	}
	svc.fetcher.mu.Unlock()

	svc.refreshPasses()

	passes := svc.GetUpcomingPasses()

	// Verify sorted by AOS.
	if !sort.SliceIsSorted(passes, func(i, j int) bool {
		return passes[i].AOS.Before(passes[j].AOS)
	}) {
		t.Error("passes not sorted by AOS")
	}

	t.Logf("Total upcoming passes: %d", len(passes))
}

func TestServiceDecodablePasses(t *testing.T) {
	catalog := map[int]*omni.CatalogEntry{
		25544: {NoradID: 25544, Name: "ISS (ZARYA)", Category: omni.CatSpaceStation, Decodable: true},
		43013: {NoradID: 43013, Name: "NOAA 20 (JPSS-1)", Category: omni.CatWeather, Decodable: false},
	}

	cachePath := filepath.Join(t.TempDir(), "tles.json")
	svc := &Service{
		fetcher: NewFetcher(catalog, cachePath, 12),
		catalog: catalog,
		minElev: 5.0,
		station: denverGS,
	}

	// Add ISS TLE.
	svc.fetcher.mu.Lock()
	svc.fetcher.tles[25544] = &TLESet{
		NoradID:   25544,
		Name:      "ISS (ZARYA)",
		Line1:     "1 25544U 98067A   24001.50000000  .00016717  00000-0  10270-3 0  9006",
		Line2:     "2 25544  51.6400 208.9163 0006703  40.5765 319.5613 15.49560532437500",
		FetchedAt: time.Now(),
	}
	svc.fetcher.mu.Unlock()

	svc.refreshPasses()

	all := svc.GetUpcomingPasses()
	decodable := svc.GetDecodablePasses()

	for _, p := range decodable {
		if !p.Decodable {
			t.Errorf("non-decodable pass in decodable results: %s", p.Name)
		}
	}

	t.Logf("All passes: %d, Decodable: %d", len(all), len(decodable))
}

func TestServiceSetStation(t *testing.T) {
	catalog := omni.BuildCatalogIndex()
	cachePath := filepath.Join(t.TempDir(), "tles.json")
	svc := &Service{
		fetcher: NewFetcher(catalog, cachePath, 12),
		catalog: catalog,
		minElev: 5.0,
	}

	// Initially no station, no passes.
	svc.refreshPasses()
	if len(svc.GetUpcomingPasses()) != 0 {
		t.Error("expected no passes without station position")
	}

	// Set station - still no TLEs loaded, so no passes.
	svc.SetStation(39.8561, -104.6737, 1609)
	if len(svc.GetUpcomingPasses()) != 0 {
		t.Log("no passes expected without TLE data")
	}
}

func TestServiceTLECount(t *testing.T) {
	catalog := omni.BuildCatalogIndex()
	cachePath := filepath.Join(t.TempDir(), "tles.json")
	svc := &Service{
		fetcher: NewFetcher(catalog, cachePath, 12),
		catalog: catalog,
		minElev: 5.0,
	}

	if svc.TLECount() != 0 {
		t.Errorf("expected 0 TLEs initially, got %d", svc.TLECount())
	}
}
