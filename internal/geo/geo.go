package geo

import "math"

const earthRadiusNM = 3440.065 // Earth radius in nautical miles

// HaversineNM returns the great-circle distance in nautical miles between
// two points given in decimal degrees.
func HaversineNM(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusNM * c
}

// Bearing returns the initial bearing (forward azimuth) in degrees from
// point 1 to point 2. Result is in the range [0, 360).
func Bearing(lat1, lon1, lat2, lon2 float64) float64 {
	lat1R := toRad(lat1)
	lat2R := toRad(lat2)
	dLon := toRad(lon2 - lon1)

	y := math.Sin(dLon) * math.Cos(lat2R)
	x := math.Cos(lat1R)*math.Sin(lat2R) -
		math.Sin(lat1R)*math.Cos(lat2R)*math.Cos(dLon)

	bearing := toDeg(math.Atan2(y, x))
	return math.Mod(bearing+360, 360)
}

// DestinationPoint returns the lat/lon of a point that is distanceNM
// nautical miles from (lat, lon) along the given bearing (degrees).
func DestinationPoint(lat, lon, bearingDeg, distanceNM float64) (float64, float64) {
	d := distanceNM / earthRadiusNM
	brng := toRad(bearingDeg)
	latR := toRad(lat)
	lonR := toRad(lon)

	lat2 := math.Asin(math.Sin(latR)*math.Cos(d) +
		math.Cos(latR)*math.Sin(d)*math.Cos(brng))
	lon2 := lonR + math.Atan2(
		math.Sin(brng)*math.Sin(d)*math.Cos(latR),
		math.Cos(d)-math.Sin(latR)*math.Sin(lat2))

	return toDeg(lat2), toDeg(lon2)
}

func toRad(deg float64) float64 { return deg * math.Pi / 180 }
func toDeg(rad float64) float64 { return rad * 180 / math.Pi }
