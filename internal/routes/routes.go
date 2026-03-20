package routes

import (
	"strings"
	"sync"
	"time"
)

// Route holds origin/destination for a callsign.
type Route struct {
	Origin      string
	Destination string
}

// Cache stores routes received from the platform ingest response.
type Cache struct {
	mu    sync.RWMutex
	cache map[string]*cacheEntry
}

type cacheEntry struct {
	route   *Route
	fetched time.Time
}

// New creates a new route cache.
func New() *Cache {
	return &Cache{
		cache: make(map[string]*cacheEntry),
	}
}

// Get returns the cached route for a callsign, or nil.
func (c *Cache) Get(callsign string) *Route {
	callsign = strings.TrimSpace(strings.ToUpper(callsign))
	if callsign == "" {
		return nil
	}

	c.mu.RLock()
	entry, ok := c.cache[callsign]
	c.mu.RUnlock()

	if ok && time.Since(entry.fetched) < 2*time.Hour {
		return entry.route
	}
	return nil
}

// Update bulk-updates the cache with routes from an ingest response.
func (c *Cache) Update(routes map[string]RouteData) {
	if len(routes) == 0 {
		return
	}
	now := time.Now()

	c.mu.Lock()
	for callsign, rd := range routes {
		cs := strings.TrimSpace(strings.ToUpper(callsign))
		if cs == "" || (rd.Origin == "" && rd.Destination == "") {
			continue
		}
		c.cache[cs] = &cacheEntry{
			route:   &Route{Origin: rd.Origin, Destination: rd.Destination},
			fetched: now,
		}
	}
	c.mu.Unlock()
}

// RouteData matches the platform's IngestResponse route format.
type RouteData struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
}
