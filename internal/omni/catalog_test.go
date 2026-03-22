package omni

import "testing"

func TestBuildCatalogIndex(t *testing.T) {
	idx := BuildCatalogIndex()

	// Should have fewer entries than DefaultCatalog due to deduplication.
	if len(idx) >= len(DefaultCatalog) {
		t.Errorf("expected deduplication to reduce entries: index=%d, catalog=%d", len(idx), len(DefaultCatalog))
	}

	// ISS should exist.
	iss, ok := idx[25544]
	if !ok {
		t.Fatal("ISS (25544) not found in index")
	}
	if iss.Name != "ISS (ZARYA)" {
		t.Errorf("ISS name = %q, want %q", iss.Name, "ISS (ZARYA)")
	}
	if iss.Category != CatSpaceStation {
		t.Errorf("ISS category = %q, want %q", iss.Category, CatSpaceStation)
	}
	if !iss.Decodable {
		t.Error("ISS should be decodable")
	}

	// NOAA 19 should be decodable weather.
	noaa19, ok := idx[33591]
	if !ok {
		t.Fatal("NOAA 19 (33591) not found in index")
	}
	if noaa19.Category != CatWeather {
		t.Errorf("NOAA 19 category = %q, want %q", noaa19.Category, CatWeather)
	}
	if !noaa19.Decodable {
		t.Error("NOAA 19 should be decodable")
	}
	if len(noaa19.Frequencies) == 0 {
		t.Error("NOAA 19 should have frequencies")
	}
}

func TestDecodableCounts(t *testing.T) {
	idx := BuildCatalogIndex()

	decodable := 0
	for _, entry := range idx {
		if entry.Decodable {
			decodable++
		}
	}

	if decodable == 0 {
		t.Error("expected at least some decodable satellites")
	}

	// Sanity check: should be fewer decodable than total.
	if decodable >= len(idx) {
		t.Errorf("expected fewer decodable (%d) than total (%d)", decodable, len(idx))
	}
}

func TestCelesTrakGroupURLs(t *testing.T) {
	urls := CelesTrakGroupURLs()
	if len(urls) == 0 {
		t.Fatal("expected CelesTrak group URLs")
	}

	expected := []string{"stations", "weather", "amateur", "cubesat", "science", "gnss", "iridium"}
	for _, group := range expected {
		if _, ok := urls[group]; !ok {
			t.Errorf("missing group %q in CelesTrak URLs", group)
		}
	}
}
