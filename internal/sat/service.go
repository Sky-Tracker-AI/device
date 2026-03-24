package sat

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/skytracker/skytracker-device/internal/omni"
)

// Service orchestrates TLE fetching and pass prediction for the device.
type Service struct {
	fetcher *Fetcher
	catalog map[int]*omni.CatalogEntry

	mu         sync.RWMutex
	station    GroundStation
	stationSet bool
	passes     []PassPrediction
	ready      bool
	minElev    float64
	refreshH   int
}

// NewService creates a satellite service with the default catalog.
// minElevDeg is the minimum elevation threshold for pass predictions.
// refreshHours controls TLE refresh interval.
func NewService(minElevDeg float64, refreshHours int) *Service {
	catalog := omni.BuildCatalogIndex()

	// Determine cache path.
	cachePath := "/var/lib/skytracker/tles.json"
	if os.Getuid() != 0 {
		home, _ := os.UserHomeDir()
		cachePath = filepath.Join(home, ".skytracker", "tles.json")
	}

	return &Service{
		fetcher:  NewFetcher(catalog, cachePath, refreshHours),
		catalog:  catalog,
		minElev:  minElevDeg,
		refreshH: refreshHours,
	}
}

// Start initializes the TLE fetcher and begins the pass prediction refresh loop.
// This method blocks until the context is cancelled.
func (s *Service) Start(ctx context.Context) {
	log.Printf("[sat] starting service: %d satellites in catalog, min elevation %.1f deg", len(s.catalog), s.minElev)

	// Re-predict passes whenever TLEs are refreshed.
	s.fetcher.onRefresh = func() {
		log.Printf("[sat] TLEs refreshed, re-predicting passes")
		s.refreshPasses()
	}

	// Start the fetcher in its own goroutine.
	fetchDone := make(chan struct{})
	go func() {
		s.fetcher.Start(ctx)
	}()

	// Wait for the fetcher to have data (poll for up to 30 seconds).
	go func() {
		defer close(fetchDone)
		deadline := time.After(30 * time.Second)
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-deadline:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				if s.fetcher.Count() > 0 {
					return
				}
			}
		}
	}()

	<-fetchDone

	tleCount := s.fetcher.Count()
	log.Printf("[sat] fetcher ready: %d TLEs loaded", tleCount)

	// Compute initial predictions if we have a station position.
	s.refreshPasses()
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()

	log.Printf("[sat] service ready: %d upcoming passes", len(s.GetUpcomingPasses()))

	// Refresh pass predictions every 30 minutes.
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshPasses()
		}
	}
}

// SetStation updates the ground station location and triggers re-prediction.
func (s *Service) SetStation(lat, lon, altM float64) {
	s.mu.Lock()
	s.station = GroundStation{Lat: lat, Lon: lon, AltM: altM}
	s.stationSet = true
	s.mu.Unlock()

	log.Printf("[sat] station updated: %.4f, %.4f (%.0fm)", lat, lon, altM)
	s.refreshPasses()
}

// refreshPasses recomputes 24-hour pass predictions for all catalog satellites.
func (s *Service) refreshPasses() {
	s.mu.RLock()
	gs := s.station
	s.mu.RUnlock()

	// Skip if no station position has been set.
	s.mu.RLock()
	set := s.stationSet
	s.mu.RUnlock()
	if !set {
		return
	}

	now := time.Now().UTC()
	var allPasses []PassPrediction

	for id, entry := range s.catalog {
		tle := s.fetcher.GetTLE(id)
		if tle == nil {
			continue
		}
		passes := PredictPasses(tle, entry, gs, now, 24, s.minElev)
		allPasses = append(allPasses, passes...)
	}

	// Sort by AOS time.
	sort.Slice(allPasses, func(i, j int) bool {
		return allPasses[i].AOS.Before(allPasses[j].AOS)
	})

	s.mu.Lock()
	s.passes = allPasses
	s.mu.Unlock()

	decodable := 0
	for _, p := range allPasses {
		if p.Decodable {
			decodable++
		}
		// Debug: log weather satellite passes.
		if p.Category == omni.CatWeather {
			log.Printf("[sat] weather pass: %s (NORAD %d) AOS=%s LOS=%s maxEl=%.0f decodable=%v",
				p.Name, p.NoradID, p.AOS.Format("15:04:05 MST"), p.LOS.Format("15:04:05 MST"), p.MaxElevation, p.Decodable)
		}
	}
	log.Printf("[sat] predicted %d passes (%d decodable) in next 24h", len(allPasses), decodable)
}

// IsReady returns true once the service has completed initial setup.
func (s *Service) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready
}

// GetUpcomingPasses returns all predicted passes sorted by AOS.
func (s *Service) GetUpcomingPasses() []PassPrediction {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Filter out passes that have already ended.
	now := time.Now().UTC()
	var upcoming []PassPrediction
	for _, p := range s.passes {
		if p.LOS.After(now) {
			upcoming = append(upcoming, p)
		}
	}
	return upcoming
}

// GetDecodablePasses returns only decodable upcoming passes.
func (s *Service) GetDecodablePasses() []PassPrediction {
	upcoming := s.GetUpcomingPasses()
	var decodable []PassPrediction
	for _, p := range upcoming {
		if p.Decodable {
			decodable = append(decodable, p)
		}
	}
	return decodable
}

// TLECount returns the number of loaded TLEs.
func (s *Service) TLECount() int {
	return s.fetcher.Count()
}

// TLEAge returns the age of the TLE cache.
func (s *Service) TLEAge() time.Duration {
	return s.fetcher.CacheAge()
}
