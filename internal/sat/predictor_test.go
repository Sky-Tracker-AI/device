package sat

import (
	"testing"
	"time"

	"github.com/skytracker/skytracker-device/internal/omni"
)

// ISS TLE for testing (epoch around 2024-01-01).
var testISSTLE = &TLESet{
	NoradID: 25544,
	Name:    "ISS (ZARYA)",
	Line1:   "1 25544U 98067A   24001.50000000  .00016717  00000-0  10270-3 0  9006",
	Line2:   "2 25544  51.6400 208.9163 0006703  40.5765 319.5613 15.49560532437500",
}

var testISSEntry = &omni.CatalogEntry{
	NoradID:     25544,
	Name:        "ISS (ZARYA)",
	Category:    omni.CatSpaceStation,
	Frequencies: []float64{145.8, 437.8},
	Decodable:   true,
	IconSize:    "large",
}

// Denver, CO ground station.
var denverGS = GroundStation{
	Lat:  39.8561,
	Lon:  -104.6737,
	AltM: 1609,
}

func TestPredictPassesISS(t *testing.T) {
	// Predict passes starting from near the TLE epoch.
	start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	passes := PredictPasses(testISSTLE, testISSEntry, denverGS, start, 24, 5.0)

	// ISS orbits ~15.5 times per day; from Denver we should see several passes in 24h.
	if len(passes) == 0 {
		t.Fatal("expected at least one ISS pass over Denver in 24 hours")
	}

	for i, pass := range passes {
		// AOS should be before LOS.
		if !pass.AOS.Before(pass.LOS) {
			t.Errorf("pass %d: AOS (%v) not before LOS (%v)", i, pass.AOS, pass.LOS)
		}

		// Max elevation should be >= min threshold.
		if pass.MaxElevation < 5.0 {
			t.Errorf("pass %d: max elevation %.1f < 5.0", i, pass.MaxElevation)
		}

		// Max elevation should be reasonable (< 90.1 degrees).
		if pass.MaxElevation > 90.1 {
			t.Errorf("pass %d: max elevation %.1f > 90", i, pass.MaxElevation)
		}

		// MaxElevTime should be between AOS and LOS.
		if pass.MaxElevTime.Before(pass.AOS) || pass.MaxElevTime.After(pass.LOS) {
			t.Errorf("pass %d: max elev time %v not between AOS %v and LOS %v", i, pass.MaxElevTime, pass.AOS, pass.LOS)
		}

		// Pass should have catalog metadata.
		if pass.NoradID != 25544 {
			t.Errorf("pass %d: NORAD ID = %d, want 25544", i, pass.NoradID)
		}
		if !pass.Decodable {
			t.Errorf("pass %d: expected decodable=true", i)
		}
		if len(pass.Frequencies) == 0 {
			t.Errorf("pass %d: expected frequencies", i)
		}
	}

	t.Logf("Found %d ISS passes over Denver in 24h", len(passes))
	for i, p := range passes {
		t.Logf("  Pass %d: AOS=%s LOS=%s MaxEl=%.1f", i+1,
			p.AOS.Format("15:04:05"), p.LOS.Format("15:04:05"), p.MaxElevation)
	}
}

func TestPredictPassesNoSatAboveThreshold(t *testing.T) {
	// Use a very high minimum elevation that likely won't be reached.
	start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	passes := PredictPasses(testISSTLE, testISSEntry, denverGS, start, 1, 89.0)

	// With 89 degree min elevation and only 1 hour, very unlikely to get a pass.
	// This is just a sanity check that the function doesn't crash.
	t.Logf("Passes with 89 deg threshold in 1h: %d", len(passes))
}

func TestPredictPassesInvalidTLE(t *testing.T) {
	// Use a properly formatted TLE with a bogus satellite number that will
	// produce a non-zero Error from go-satellite's TLEToSat.
	badTLE := &TLESet{
		NoradID: 99999,
		Name:    "BAD SAT",
		Line1:   "1 99999U 00000A   24001.00000000  .00000000  00000+0  00000+0 0  0000",
		Line2:   "2 99999   0.0000   0.0000 9999999   0.0000   0.0000  0.00000000    00",
	}
	entry := &omni.CatalogEntry{
		NoradID:  99999,
		Name:     "BAD SAT",
		Category: omni.CatOther,
	}

	start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	passes := PredictPasses(badTLE, entry, denverGS, start, 1, 5.0)

	// With a zero orbital period, propagation should fail or produce no valid passes.
	t.Logf("Passes for invalid TLE: %d", len(passes))
}
