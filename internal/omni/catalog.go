package omni

// SatCategory classifies satellites for display and filtering.
type SatCategory string

const (
	CatWeather      SatCategory = "weather"
	CatSpaceStation SatCategory = "space_station"
	CatAmateur      SatCategory = "amateur"
	CatCubesat      SatCategory = "cubesat"
	CatNavigation   SatCategory = "navigation"
	CatScience      SatCategory = "science"
	CatComm         SatCategory = "comm"
	CatOther        SatCategory = "other"
)

// CatalogEntry describes a satellite of interest to SkyTracker users.
type CatalogEntry struct {
	NoradID     int         `json:"norad_id"`
	Name        string      `json:"name"`
	Category    SatCategory `json:"category"`
	Frequencies []float64   `json:"frequencies"` // MHz
	Decodable   bool        `json:"decodable"`   // true if SkyTracker hardware can decode
	IconSize    string      `json:"icon_size"`   // "large", "medium", "small"
}

// DefaultCatalog is the curated list of interesting satellites.
var DefaultCatalog = []CatalogEntry{
	// -- Space Stations --
	{NoradID: 25544, Name: "ISS (ZARYA)", Category: CatSpaceStation, Frequencies: []float64{145.8, 437.8}, Decodable: true, IconSize: "large"},
	{NoradID: 48274, Name: "TIANGONG (CSS)", Category: CatSpaceStation, Frequencies: []float64{145.825}, Decodable: false, IconSize: "large"},

	// -- Weather Satellites (LRPT decodable on 137 MHz) --
	// Only METEOR-M N2-3 and N2-4 remain active on 137 MHz as of 2025.
	// NOAA 15/18/19 APT decommissioned Jun-Aug 2025. METEOR-M N2 dead Dec 2022. N2-2 LRPT failed.
	{NoradID: 57166, Name: "METEOR-M N2-3", Category: CatWeather, Frequencies: []float64{137.9}, Decodable: true, IconSize: "large"},
	{NoradID: 59051, Name: "METEOR-M N2-4", Category: CatWeather, Frequencies: []float64{137.9}, Decodable: true, IconSize: "large"},
	// Decommissioned — tracked for visibility only
	{NoradID: 25338, Name: "NOAA 15", Category: CatWeather, Frequencies: []float64{137.62}, Decodable: false, IconSize: "medium"},
	{NoradID: 28654, Name: "NOAA 18", Category: CatWeather, Frequencies: []float64{137.9125}, Decodable: false, IconSize: "medium"},
	{NoradID: 33591, Name: "NOAA 19", Category: CatWeather, Frequencies: []float64{137.1}, Decodable: false, IconSize: "medium"},
	{NoradID: 43013, Name: "NOAA 20 (JPSS-1)", Category: CatWeather, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 54234, Name: "NOAA 21 (JPSS-2)", Category: CatWeather, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 25994, Name: "TERRA", Category: CatWeather, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 27424, Name: "AQUA", Category: CatWeather, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 37849, Name: "SUOMI NPP", Category: CatWeather, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 29499, Name: "METOP-A", Category: CatWeather, Frequencies: []float64{137.1}, Decodable: true, IconSize: "medium"},
	{NoradID: 38771, Name: "METOP-B", Category: CatWeather, Frequencies: []float64{137.1}, Decodable: true, IconSize: "medium"},
	{NoradID: 56217, Name: "METOP-C", Category: CatWeather, Frequencies: []float64{137.1}, Decodable: true, IconSize: "medium"},
	{NoradID: 43689, Name: "FENGYUN 3D", Category: CatWeather, Frequencies: []float64{137.025, 137.75}, Decodable: true, IconSize: "medium"},
	{NoradID: 49008, Name: "FENGYUN 3E", Category: CatWeather, Frequencies: []float64{137.025}, Decodable: true, IconSize: "medium"},

	// -- Amateur Radio Satellites --
	{NoradID: 43017, Name: "AO-91 (FOX-1B)", Category: CatAmateur, Frequencies: []float64{145.96, 435.25}, Decodable: true, IconSize: "medium"},
	{NoradID: 43137, Name: "AO-92 (FOX-1D)", Category: CatAmateur, Frequencies: []float64{145.88, 435.35}, Decodable: true, IconSize: "medium"},
	{NoradID: 27607, Name: "SO-50 (SAUDISAT-1C)", Category: CatAmateur, Frequencies: []float64{145.85, 436.795}, Decodable: true, IconSize: "medium"},
	{NoradID: 30776, Name: "FALCONSAT-3", Category: CatAmateur, Frequencies: []float64{435.103}, Decodable: true, IconSize: "medium"},
	{NoradID: 42761, Name: "CAS-4A (ZHUHAI-1 01)", Category: CatAmateur, Frequencies: []float64{145.855, 145.91}, Decodable: true, IconSize: "medium"},
	{NoradID: 42759, Name: "CAS-4B (ZHUHAI-1 02)", Category: CatAmateur, Frequencies: []float64{145.875, 145.925}, Decodable: true, IconSize: "medium"},
	{NoradID: 44832, Name: "AO-95 (FOX-1CLIFF)", Category: CatAmateur, Frequencies: []float64{145.92, 435.15}, Decodable: true, IconSize: "medium"},
	{NoradID: 47311, Name: "AO-109 (FOX-1E)", Category: CatAmateur, Frequencies: []float64{145.88, 435.75}, Decodable: true, IconSize: "medium"},
	{NoradID: 7530, Name: "AO-7", Category: CatAmateur, Frequencies: []float64{145.9775, 29.502}, Decodable: true, IconSize: "medium"},
	{NoradID: 14781, Name: "UO-11 (UOSAT-2)", Category: CatAmateur, Frequencies: []float64{145.825, 435.025}, Decodable: true, IconSize: "medium"},
	{NoradID: 24278, Name: "FO-29 (JAS-2)", Category: CatAmateur, Frequencies: []float64{145.9, 435.8}, Decodable: true, IconSize: "medium"},
	{NoradID: 40908, Name: "CAS-3H (LILAC-1)", Category: CatAmateur, Frequencies: []float64{437.2}, Decodable: true, IconSize: "medium"},
	{NoradID: 40967, Name: "XW-2A", Category: CatAmateur, Frequencies: []float64{145.665, 435.03}, Decodable: true, IconSize: "medium"},
	{NoradID: 40970, Name: "XW-2C", Category: CatAmateur, Frequencies: []float64{145.795, 435.15}, Decodable: true, IconSize: "medium"},
	{NoradID: 40971, Name: "XW-2D", Category: CatAmateur, Frequencies: []float64{145.86, 435.215}, Decodable: true, IconSize: "medium"},
	{NoradID: 40906, Name: "XW-2F", Category: CatAmateur, Frequencies: []float64{145.985, 435.375}, Decodable: true, IconSize: "medium"},
	{NoradID: 43678, Name: "PO-101 (DIWATA-2)", Category: CatAmateur, Frequencies: []float64{145.9, 437.5}, Decodable: true, IconSize: "medium"},
	{NoradID: 46494, Name: "RS-44 (DOSAAF-85)", Category: CatAmateur, Frequencies: []float64{145.935, 435.61}, Decodable: true, IconSize: "medium"},
	{NoradID: 43770, Name: "FUNCUBE-1 (AO-73)", Category: CatAmateur, Frequencies: []float64{145.935, 435.15}, Decodable: true, IconSize: "medium"},
	{NoradID: 43880, Name: "JY1SAT (JO-97)", Category: CatAmateur, Frequencies: []float64{145.84, 435.1}, Decodable: true, IconSize: "medium"},
	{NoradID: 44909, Name: "TEVEL-1", Category: CatAmateur, Frequencies: []float64{436.4}, Decodable: true, IconSize: "small"},
	{NoradID: 44910, Name: "TEVEL-2", Category: CatAmateur, Frequencies: []float64{436.4}, Decodable: true, IconSize: "small"},
	{NoradID: 44911, Name: "TEVEL-3", Category: CatAmateur, Frequencies: []float64{436.4}, Decodable: true, IconSize: "small"},
	{NoradID: 44912, Name: "TEVEL-4", Category: CatAmateur, Frequencies: []float64{436.4}, Decodable: true, IconSize: "small"},
	{NoradID: 44913, Name: "TEVEL-5", Category: CatAmateur, Frequencies: []float64{436.4}, Decodable: true, IconSize: "small"},
	{NoradID: 51069, Name: "GREENCUBE", Category: CatAmateur, Frequencies: []float64{435.31}, Decodable: true, IconSize: "small"},
	{NoradID: 47438, Name: "UVSQ-SAT", Category: CatAmateur, Frequencies: []float64{437.02}, Decodable: true, IconSize: "small"},

	// -- CubeSats --
	{NoradID: 43803, Name: "ICEYE-X1", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44420, Name: "SPACEBEE-8", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44421, Name: "SPACEBEE-9", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 43793, Name: "LEMUR-2-JOEL", Category: CatCubesat, Frequencies: []float64{137.0, 400.0}, Decodable: false, IconSize: "small"},
	{NoradID: 43794, Name: "LEMUR-2-LYNSEY-SYMO", Category: CatCubesat, Frequencies: []float64{137.0, 400.0}, Decodable: false, IconSize: "small"},
	{NoradID: 43795, Name: "LEMUR-2-ORZORO", Category: CatCubesat, Frequencies: []float64{137.0, 400.0}, Decodable: false, IconSize: "small"},
	{NoradID: 44406, Name: "MOVE-II", Category: CatCubesat, Frequencies: []float64{145.95}, Decodable: true, IconSize: "small"},
	{NoradID: 43792, Name: "CORVUS-BC3", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 46287, Name: "FORESAIL-1", Category: CatCubesat, Frequencies: []float64{437.125}, Decodable: true, IconSize: "small"},
	{NoradID: 43614, Name: "RANGE-A", Category: CatCubesat, Frequencies: []float64{401.0}, Decodable: false, IconSize: "small"},
	{NoradID: 43615, Name: "RANGE-B", Category: CatCubesat, Frequencies: []float64{401.0}, Decodable: false, IconSize: "small"},
	{NoradID: 47960, Name: "CUAVA-1", Category: CatCubesat, Frequencies: []float64{437.075}, Decodable: true, IconSize: "small"},
	{NoradID: 44352, Name: "LUCKY-7", Category: CatCubesat, Frequencies: []float64{437.525}, Decodable: true, IconSize: "small"},
	{NoradID: 43774, Name: "ICEYE-X2", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 43616, Name: "CENTAURI-1", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44365, Name: "BEESAT-9", Category: CatCubesat, Frequencies: []float64{435.95}, Decodable: true, IconSize: "small"},
	{NoradID: 44878, Name: "OPS-SAT", Category: CatCubesat, Frequencies: []float64{437.2}, Decodable: true, IconSize: "small"},
	{NoradID: 47951, Name: "UPMSAT-2", Category: CatCubesat, Frequencies: []float64{437.405}, Decodable: true, IconSize: "small"},
	{NoradID: 43786, Name: "NETSAT 1", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 43787, Name: "NETSAT 2", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 43788, Name: "NETSAT 3", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 43789, Name: "NETSAT 4", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 47963, Name: "EASAT-2", Category: CatCubesat, Frequencies: []float64{436.666}, Decodable: true, IconSize: "small"},
	{NoradID: 47964, Name: "HADES", Category: CatCubesat, Frequencies: []float64{436.888}, Decodable: true, IconSize: "small"},
	{NoradID: 46839, Name: "NORSAT-3", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 43802, Name: "AISTECHSAT-2", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44830, Name: "EXOLAUNCH ECOD", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44831, Name: "TRSI-SAT", Category: CatCubesat, Frequencies: []float64{435.275}, Decodable: true, IconSize: "small"},
	{NoradID: 44426, Name: "SPACEBEE-10", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44427, Name: "SPACEBEE-11", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44428, Name: "SPACEBEE-12", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 51074, Name: "MTCube-2", Category: CatCubesat, Frequencies: []float64{436.95}, Decodable: true, IconSize: "small"},
	{NoradID: 51082, Name: "ALSAT#1", Category: CatCubesat, Frequencies: []float64{436.91}, Decodable: true, IconSize: "small"},
	{NoradID: 44429, Name: "ASTROCAST-0.1", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 47952, Name: "LEDSAT", Category: CatCubesat, Frequencies: []float64{435.19}, Decodable: true, IconSize: "small"},
	{NoradID: 47957, Name: "DIY-1", Category: CatCubesat, Frequencies: []float64{437.05}, Decodable: true, IconSize: "small"},
	{NoradID: 44829, Name: "REAKTOR HELLO WORLD", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 51085, Name: "CELESTA", Category: CatCubesat, Frequencies: []float64{401.0}, Decodable: false, IconSize: "small"},
	{NoradID: 44419, Name: "SPACEBEE-7", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44414, Name: "SPACEBEE-5", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 51084, Name: "GAMASAT", Category: CatCubesat, Frequencies: []float64{436.0}, Decodable: true, IconSize: "small"},
	{NoradID: 44415, Name: "SPACEBEE-6", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44346, Name: "EXOLAUNCH ECOD 1", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},

	// -- Navigation Satellites --
	{NoradID: 28474, Name: "GPS IIR-14 (PRN 11)", Category: CatNavigation, Frequencies: []float64{1575.42, 1227.6}, Decodable: false, IconSize: "medium"},
	{NoradID: 28874, Name: "GPS IIR-15 (PRN 28)", Category: CatNavigation, Frequencies: []float64{1575.42, 1227.6}, Decodable: false, IconSize: "medium"},
	{NoradID: 32260, Name: "GPS IIR-M-3 (PRN 15)", Category: CatNavigation, Frequencies: []float64{1575.42, 1227.6, 1176.45}, Decodable: false, IconSize: "medium"},
	{NoradID: 29601, Name: "GPS IIR-M-1 (PRN 17)", Category: CatNavigation, Frequencies: []float64{1575.42, 1227.6, 1176.45}, Decodable: false, IconSize: "medium"},
	{NoradID: 37753, Name: "GPS IIF-2 (PRN 25)", Category: CatNavigation, Frequencies: []float64{1575.42, 1227.6, 1176.45}, Decodable: false, IconSize: "medium"},
	{NoradID: 39166, Name: "GPS IIF-4 (PRN 27)", Category: CatNavigation, Frequencies: []float64{1575.42, 1227.6, 1176.45}, Decodable: false, IconSize: "medium"},
	{NoradID: 40534, Name: "GPS IIF-7 (PRN 08)", Category: CatNavigation, Frequencies: []float64{1575.42, 1227.6, 1176.45}, Decodable: false, IconSize: "medium"},
	{NoradID: 41019, Name: "GPS IIF-10 (PRN 32)", Category: CatNavigation, Frequencies: []float64{1575.42, 1227.6, 1176.45}, Decodable: false, IconSize: "medium"},
	{NoradID: 43873, Name: "GPS III SV01 (PRN 04)", Category: CatNavigation, Frequencies: []float64{1575.42, 1227.6, 1176.45}, Decodable: false, IconSize: "medium"},
	{NoradID: 44506, Name: "GPS III SV02 (PRN 18)", Category: CatNavigation, Frequencies: []float64{1575.42, 1227.6, 1176.45}, Decodable: false, IconSize: "medium"},
	{NoradID: 28922, Name: "GALILEO-PFM", Category: CatNavigation, Frequencies: []float64{1575.42, 1176.45}, Decodable: false, IconSize: "medium"},
	{NoradID: 32781, Name: "GALILEO-FM2", Category: CatNavigation, Frequencies: []float64{1575.42, 1176.45}, Decodable: false, IconSize: "medium"},
	{NoradID: 32384, Name: "GLONASS-M 724", Category: CatNavigation, Frequencies: []float64{1602.0, 1246.0}, Decodable: false, IconSize: "medium"},
	{NoradID: 36111, Name: "GLONASS-M 730", Category: CatNavigation, Frequencies: []float64{1602.0, 1246.0}, Decodable: false, IconSize: "medium"},

	// -- Science Satellites --
	{NoradID: 20580, Name: "HUBBLE SPACE TELESCOPE", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "large"},
	{NoradID: 36395, Name: "SDO (SOLAR DYNAMICS OBS)", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 27386, Name: "INTEGRAL", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 44874, Name: "CHEOPS", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 43435, Name: "TESS", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 39084, Name: "LANDSAT 8", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 49260, Name: "LANDSAT 9", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 36508, Name: "CRYOSAT-2", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 38337, Name: "NUSTAR", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 43205, Name: "ICESAT-2", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 27424, Name: "AQUA (EOS PM-1)", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 25063, Name: "SWAS", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44071, Name: "PRISMA", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 43613, Name: "AEOLUS", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 38755, Name: "SARAL", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 39634, Name: "GRACE-FO 1", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 39635, Name: "GRACE-FO 2", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},

	// -- Communication Satellites --
	{NoradID: 25544, Name: "ISS (ZARYA)", Category: CatSpaceStation, Frequencies: []float64{145.8, 437.8}, Decodable: true, IconSize: "large"}, // duplicate filtered by index
	{NoradID: 36516, Name: "ORBCOMM FM109", Category: CatComm, Frequencies: []float64{137.0}, Decodable: false, IconSize: "small"},
	{NoradID: 36517, Name: "ORBCOMM FM107", Category: CatComm, Frequencies: []float64{137.0}, Decodable: false, IconSize: "small"},
	{NoradID: 40086, Name: "ORBCOMM FM106", Category: CatComm, Frequencies: []float64{137.0}, Decodable: false, IconSize: "small"},
	{NoradID: 40087, Name: "ORBCOMM FM111", Category: CatComm, Frequencies: []float64{137.0}, Decodable: false, IconSize: "small"},
	{NoradID: 40090, Name: "ORBCOMM FM113", Category: CatComm, Frequencies: []float64{137.0}, Decodable: false, IconSize: "small"},
	{NoradID: 40091, Name: "ORBCOMM FM115", Category: CatComm, Frequencies: []float64{137.0}, Decodable: false, IconSize: "small"},
	{NoradID: 41187, Name: "IRIDIUM 106", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 41188, Name: "IRIDIUM 103", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 41189, Name: "IRIDIUM 109", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 41190, Name: "IRIDIUM 102", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 41191, Name: "IRIDIUM 105", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 41192, Name: "IRIDIUM 104", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42955, Name: "IRIDIUM 148", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42956, Name: "IRIDIUM 150", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42957, Name: "IRIDIUM 153", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42958, Name: "IRIDIUM 154", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42959, Name: "IRIDIUM 155", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42960, Name: "IRIDIUM 156", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42961, Name: "IRIDIUM 151", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42962, Name: "IRIDIUM 157", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42963, Name: "IRIDIUM 152", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42964, Name: "IRIDIUM 158", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 36585, Name: "GLOBALSTAR M087", Category: CatComm, Frequencies: []float64{1610.0}, Decodable: false, IconSize: "small"},
	{NoradID: 36586, Name: "GLOBALSTAR M088", Category: CatComm, Frequencies: []float64{1610.0}, Decodable: false, IconSize: "small"},
	{NoradID: 36587, Name: "GLOBALSTAR M091", Category: CatComm, Frequencies: []float64{1610.0}, Decodable: false, IconSize: "small"},
	{NoradID: 36588, Name: "GLOBALSTAR M085", Category: CatComm, Frequencies: []float64{1610.0}, Decodable: false, IconSize: "small"},
	{NoradID: 36589, Name: "GLOBALSTAR M081", Category: CatComm, Frequencies: []float64{1610.0}, Decodable: false, IconSize: "small"},
	{NoradID: 36590, Name: "GLOBALSTAR M089", Category: CatComm, Frequencies: []float64{1610.0}, Decodable: false, IconSize: "small"},

	// -- More Iridium NEXT --
	{NoradID: 42965, Name: "IRIDIUM 159", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42966, Name: "IRIDIUM 160", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42967, Name: "IRIDIUM 149", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 42968, Name: "IRIDIUM 161", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 43249, Name: "IRIDIUM 163", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 43250, Name: "IRIDIUM 168", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 43251, Name: "IRIDIUM 169", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 43252, Name: "IRIDIUM 164", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 43253, Name: "IRIDIUM 166", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},
	{NoradID: 43254, Name: "IRIDIUM 170", Category: CatComm, Frequencies: []float64{1616.0}, Decodable: false, IconSize: "small"},

	// -- More Globalstar --
	{NoradID: 36591, Name: "GLOBALSTAR M083", Category: CatComm, Frequencies: []float64{1610.0}, Decodable: false, IconSize: "small"},
	{NoradID: 36592, Name: "GLOBALSTAR M090", Category: CatComm, Frequencies: []float64{1610.0}, Decodable: false, IconSize: "small"},
	{NoradID: 38087, Name: "GLOBALSTAR M093", Category: CatComm, Frequencies: []float64{1610.0}, Decodable: false, IconSize: "small"},
	{NoradID: 38088, Name: "GLOBALSTAR M094", Category: CatComm, Frequencies: []float64{1610.0}, Decodable: false, IconSize: "small"},
	{NoradID: 38089, Name: "GLOBALSTAR M096", Category: CatComm, Frequencies: []float64{1610.0}, Decodable: false, IconSize: "small"},
	{NoradID: 38090, Name: "GLOBALSTAR M082", Category: CatComm, Frequencies: []float64{1610.0}, Decodable: false, IconSize: "small"},

	// -- More Amateur / Education Satellites --
	{NoradID: 43017, Name: "RADFXSAT (FOX-1B)", Category: CatAmateur, Frequencies: []float64{145.96}, Decodable: true, IconSize: "small"}, // dup filtered
	{NoradID: 47954, Name: "SANOSAT-1", Category: CatAmateur, Frequencies: []float64{436.0}, Decodable: true, IconSize: "small"},
	{NoradID: 47955, Name: "FEES", Category: CatAmateur, Frequencies: []float64{435.0}, Decodable: true, IconSize: "small"},
	{NoradID: 47956, Name: "STECCO", Category: CatAmateur, Frequencies: []float64{435.0}, Decodable: true, IconSize: "small"},
	{NoradID: 47959, Name: "SRCUBE", Category: CatAmateur, Frequencies: []float64{437.0}, Decodable: true, IconSize: "small"},
	{NoradID: 47961, Name: "BOBCAT-1", Category: CatAmateur, Frequencies: []float64{437.325}, Decodable: true, IconSize: "small"},
	{NoradID: 51073, Name: "CELESTA", Category: CatAmateur, Frequencies: []float64{401.0}, Decodable: false, IconSize: "small"},
	{NoradID: 51081, Name: "KITSUNE", Category: CatAmateur, Frequencies: []float64{435.0}, Decodable: true, IconSize: "small"},
	{NoradID: 51086, Name: "ALPHA", Category: CatAmateur, Frequencies: []float64{435.0}, Decodable: true, IconSize: "small"},

	// -- More CubeSats --
	{NoradID: 44400, Name: "SPOC", Category: CatCubesat, Frequencies: []float64{437.2}, Decodable: true, IconSize: "small"},
	{NoradID: 44401, Name: "SWIATOWID", Category: CatCubesat, Frequencies: []float64{435.5}, Decodable: true, IconSize: "small"},
	{NoradID: 44413, Name: "SPACEBEE-4", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44347, Name: "NSAT", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44348, Name: "CORVUS-BC 2", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44349, Name: "LEMUR-2-TALLHAMN", Category: CatCubesat, Frequencies: []float64{137.0}, Decodable: false, IconSize: "small"},
	{NoradID: 44350, Name: "LEMUR-2-SATCHMO", Category: CatCubesat, Frequencies: []float64{137.0}, Decodable: false, IconSize: "small"},
	{NoradID: 44351, Name: "LEMUR-2-MCPEAKE", Category: CatCubesat, Frequencies: []float64{137.0}, Decodable: false, IconSize: "small"},
	{NoradID: 44353, Name: "MYSAT-1", Category: CatCubesat, Frequencies: []float64{436.0}, Decodable: true, IconSize: "small"},
	{NoradID: 44355, Name: "ATL-1", Category: CatCubesat, Frequencies: []float64{437.175}, Decodable: true, IconSize: "small"},
	{NoradID: 44356, Name: "SMOG-P", Category: CatCubesat, Frequencies: []float64{437.15}, Decodable: true, IconSize: "small"},
	{NoradID: 44357, Name: "NOOR 1A", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44358, Name: "NOOR 1B", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44360, Name: "PAINANI-2", Category: CatCubesat, Frequencies: []float64{437.475}, Decodable: true, IconSize: "small"},
	{NoradID: 44362, Name: "EXOLAUNCH ECOD 3", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44367, Name: "D-SAT", Category: CatCubesat, Frequencies: []float64{}, Decodable: false, IconSize: "small"},

	// -- More Science --
	{NoradID: 43437, Name: "SENTINEL-3B", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 39159, Name: "SENTINEL-1A", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 41456, Name: "SENTINEL-1B", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 40697, Name: "SENTINEL-2A", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 42063, Name: "SENTINEL-2B", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 41335, Name: "SENTINEL-3A", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 48899, Name: "SENTINEL-6A (MICHAEL FREILICH)", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 43600, Name: "AEOLUS", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 38077, Name: "PROBA-V", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 36036, Name: "PROBA-2", Category: CatScience, Frequencies: []float64{}, Decodable: false, IconSize: "small"},

	// -- More Navigation --
	{NoradID: 37846, Name: "BEIDOU-M3", Category: CatNavigation, Frequencies: []float64{1561.098, 1207.14}, Decodable: false, IconSize: "medium"},
	{NoradID: 37256, Name: "BEIDOU-M1", Category: CatNavigation, Frequencies: []float64{1561.098, 1207.14}, Decodable: false, IconSize: "medium"},
	{NoradID: 40549, Name: "BEIDOU-3 M1-S", Category: CatNavigation, Frequencies: []float64{1561.098, 1207.14}, Decodable: false, IconSize: "medium"},
	{NoradID: 40748, Name: "BEIDOU-3 M2-S", Category: CatNavigation, Frequencies: []float64{1561.098, 1207.14}, Decodable: false, IconSize: "medium"},
	{NoradID: 44204, Name: "GPS III SV03 (PRN 11)", Category: CatNavigation, Frequencies: []float64{1575.42, 1227.6, 1176.45}, Decodable: false, IconSize: "medium"},
	{NoradID: 48859, Name: "GPS III SV05 (PRN 28)", Category: CatNavigation, Frequencies: []float64{1575.42, 1227.6, 1176.45}, Decodable: false, IconSize: "medium"},

	// -- Other Notable Objects --
	{NoradID: 36411, Name: "TIANLIAN 1-02", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 48275, Name: "TIANHE MODULE", Category: CatSpaceStation, Frequencies: []float64{}, Decodable: false, IconSize: "large"},
	{NoradID: 56756, Name: "WENTIAN MODULE", Category: CatSpaceStation, Frequencies: []float64{}, Decodable: false, IconSize: "large"},
	{NoradID: 54216, Name: "MENGTIAN MODULE", Category: CatSpaceStation, Frequencies: []float64{}, Decodable: false, IconSize: "large"},
	{NoradID: 56258, Name: "CREW DRAGON ENDURANCE", Category: CatSpaceStation, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},
	{NoradID: 49044, Name: "NAUKA (MLM)", Category: CatSpaceStation, Frequencies: []float64{}, Decodable: false, IconSize: "medium"},

	// -- GOES / Geostationary Weather --
	{NoradID: 29155, Name: "GOES 13", Category: CatWeather, Frequencies: []float64{1694.1}, Decodable: false, IconSize: "medium"},
	{NoradID: 36441, Name: "GOES 15", Category: CatWeather, Frequencies: []float64{1694.1}, Decodable: false, IconSize: "medium"},
	{NoradID: 41866, Name: "GOES 16", Category: CatWeather, Frequencies: []float64{1694.1}, Decodable: false, IconSize: "medium"},
	{NoradID: 43226, Name: "GOES 17", Category: CatWeather, Frequencies: []float64{1694.1}, Decodable: false, IconSize: "medium"},
	{NoradID: 51850, Name: "GOES 18", Category: CatWeather, Frequencies: []float64{1694.1}, Decodable: false, IconSize: "medium"},

	// -- Starlink (representative bright early sats) --
	{NoradID: 44238, Name: "STARLINK-1008", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44239, Name: "STARLINK-1009", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44240, Name: "STARLINK-1010", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44241, Name: "STARLINK-1011", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44242, Name: "STARLINK-1012", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44243, Name: "STARLINK-1013", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44244, Name: "STARLINK-1014", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44245, Name: "STARLINK-1015", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44246, Name: "STARLINK-1016", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44247, Name: "STARLINK-1017", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},

	// -- OneWeb --
	{NoradID: 44058, Name: "ONEWEB-0012", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44059, Name: "ONEWEB-0010", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44060, Name: "ONEWEB-0008", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44061, Name: "ONEWEB-0007", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44062, Name: "ONEWEB-0006", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
	{NoradID: 44063, Name: "ONEWEB-0011", Category: CatComm, Frequencies: []float64{}, Decodable: false, IconSize: "small"},
}

// CelesTrakGroupURLs returns the CelesTrak TLE text URLs keyed by group name.
func CelesTrakGroupURLs() map[string]string {
	return map[string]string{
		"stations":   "https://celestrak.org/NORAD/elements/gp.php?GROUP=stations&FORMAT=tle",
		"weather":    "https://celestrak.org/NORAD/elements/gp.php?GROUP=weather&FORMAT=tle",
		"amateur":    "https://celestrak.org/NORAD/elements/gp.php?GROUP=amateur&FORMAT=tle",
		"cubesat":    "https://celestrak.org/NORAD/elements/gp.php?GROUP=cubesat&FORMAT=tle",
		"science":    "https://celestrak.org/NORAD/elements/gp.php?GROUP=science&FORMAT=tle",
		"tle-new":    "https://celestrak.org/NORAD/elements/gp.php?GROUP=tle-new&FORMAT=tle",
		"gnss":       "https://celestrak.org/NORAD/elements/gp.php?GROUP=gnss&FORMAT=tle",
		"iridium":    "https://celestrak.org/NORAD/elements/gp.php?GROUP=iridium-NEXT&FORMAT=tle",
		"orbcomm":    "https://celestrak.org/NORAD/elements/gp.php?GROUP=orbcomm&FORMAT=tle",
		"globalstar": "https://celestrak.org/NORAD/elements/gp.php?GROUP=globalstar&FORMAT=tle",
		"noaa":       "https://celestrak.org/NORAD/elements/gp.php?GROUP=noaa&FORMAT=tle",
		"resource":   "https://celestrak.org/NORAD/elements/gp.php?GROUP=resource&FORMAT=tle",
		"starlink":   "https://celestrak.org/NORAD/elements/gp.php?GROUP=starlink&FORMAT=tle",
		"oneweb":     "https://celestrak.org/NORAD/elements/gp.php?GROUP=oneweb&FORMAT=tle",
	}
}

// BuildCatalogIndex returns a map of NORAD ID to CatalogEntry for fast lookup.
// Duplicate NORAD IDs in the catalog are silently deduplicated (first wins).
func BuildCatalogIndex() map[int]*CatalogEntry {
	idx := make(map[int]*CatalogEntry, len(DefaultCatalog))
	for i := range DefaultCatalog {
		if _, exists := idx[DefaultCatalog[i].NoradID]; !exists {
			idx[DefaultCatalog[i].NoradID] = &DefaultCatalog[i]
		}
	}
	return idx
}
