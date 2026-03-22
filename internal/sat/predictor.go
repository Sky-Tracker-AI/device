package sat

import (
	"log"
	"math"
	"time"

	satellite "github.com/joshuaferrara/go-satellite"

	"github.com/skytracker/skytracker-device/internal/omni"
)

// PredictPasses finds all passes of a satellite over a ground station within
// the given time window. It steps through time in 30-second increments and
// detects AOS/LOS transitions across the elevation threshold.
func PredictPasses(tle *TLESet, entry *omni.CatalogEntry, gs GroundStation, start time.Time, hours int, minElevDeg float64) []PassPrediction {
	if hours <= 0 {
		hours = 24
	}

	step := 30 * time.Second
	end := start.Add(time.Duration(hours) * time.Hour)

	sat := satellite.TLEToSat(tle.Line1, tle.Line2, satellite.GravityWGS84)
	if sat.Error != 0 {
		log.Printf("[sat] TLE parse error for NORAD %d: error code %d", tle.NoradID, sat.Error)
		return nil
	}

	obsCoords := satellite.LatLong{
		Latitude:  gs.Lat * math.Pi / 180.0,
		Longitude: gs.Lon * math.Pi / 180.0,
	}
	obsAltKm := gs.AltM / 1000.0

	var passes []PassPrediction
	inPass := false
	var currentPass PassPrediction
	var maxEl float64

	for t := start; !t.After(end); t = t.Add(step) {
		year, month, day := t.Date()
		hour, min, sec := t.Clock()

		position, _ := satellite.Propagate(sat, year, int(month), day, hour, min, sec)

		// Skip failed propagations.
		if position.X == 0 && position.Y == 0 && position.Z == 0 {
			continue
		}

		jday := satellite.JDay(year, int(month), day, hour, min, sec)
		lookAngles := satellite.ECIToLookAngles(position, obsCoords, obsAltKm, jday)
		elDeg := lookAngles.El * 180.0 / math.Pi

		aboveHorizon := elDeg >= minElevDeg

		if aboveHorizon && !inPass {
			// AOS - start of pass.
			inPass = true
			maxEl = elDeg
			currentPass = PassPrediction{
				NoradID:      tle.NoradID,
				Name:         entry.Name,
				Category:     entry.Category,
				AOS:          t,
				MaxElevation: elDeg,
				MaxElevTime:  t,
				Decodable:    entry.Decodable,
				Frequencies:  entry.Frequencies,
			}
		} else if aboveHorizon && inPass {
			// Mid-pass - track max elevation.
			if elDeg > maxEl {
				maxEl = elDeg
				currentPass.MaxElevation = elDeg
				currentPass.MaxElevTime = t
			}
		} else if !aboveHorizon && inPass {
			// LOS - end of pass.
			inPass = false
			currentPass.LOS = t
			passes = append(passes, currentPass)
		}
	}

	// If still in pass at end of window, close it.
	if inPass {
		currentPass.LOS = end
		passes = append(passes, currentPass)
	}

	return passes
}
