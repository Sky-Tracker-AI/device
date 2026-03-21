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

const cacheTTL = 2 * time.Hour

// Cache stores routes received from the platform ingest response.
type Cache struct {
	mu    sync.RWMutex
	cache map[string]*cacheEntry
	stop  chan struct{}
}

type cacheEntry struct {
	route   *Route
	fetched time.Time
}

// New creates a new route cache and starts a background goroutine that
// evicts stale entries every 30 minutes.
func New() *Cache {
	c := &Cache{
		cache: make(map[string]*cacheEntry),
		stop:  make(chan struct{}),
	}
	go c.evictLoop()
	return c
}

// Close stops the background eviction goroutine.
func (c *Cache) Close() {
	close(c.stop)
}

// evictLoop removes entries older than cacheTTL every 30 minutes.
func (c *Cache) evictLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.evictStale()
		case <-c.stop:
			return
		}
	}
}

// evictStale removes all entries older than cacheTTL.
func (c *Cache) evictStale() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for key, entry := range c.cache {
		if now.Sub(entry.fetched) >= cacheTTL {
			delete(c.cache, key)
		}
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

	if ok && time.Since(entry.fetched) < cacheTTL {
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
