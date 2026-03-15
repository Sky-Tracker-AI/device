package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Route holds origin/destination for a callsign.
type Route struct {
	Origin      string // IATA code, e.g. "AUS"
	OriginName  string // e.g. "Austin-Bergstrom International Airport"
	Destination string // IATA code, e.g. "ABQ"
	DestName    string // e.g. "Albuquerque International Sunport"
}

// Lookup caches route lookups from the adsbdb.com API.
type Lookup struct {
	client *http.Client

	mu    sync.RWMutex
	cache map[string]*cacheEntry
}

type cacheEntry struct {
	route   *Route
	fetched time.Time
}

// New creates a new route lookup.
func New() *Lookup {
	return &Lookup{
		client: &http.Client{Timeout: 5 * time.Second},
		cache:  make(map[string]*cacheEntry),
	}
}

// Get returns the route for a callsign, fetching from the API if needed.
// Returns nil if no route is found or the callsign is empty/GA.
func (l *Lookup) Get(callsign string) *Route {
	callsign = strings.TrimSpace(strings.ToUpper(callsign))
	if callsign == "" {
		return nil
	}
	// Skip N-numbers (GA aircraft don't have routes).
	if len(callsign) > 1 && callsign[0] == 'N' && callsign[1] >= '0' && callsign[1] <= '9' {
		return nil
	}

	l.mu.RLock()
	entry, ok := l.cache[callsign]
	l.mu.RUnlock()

	if ok {
		// Cache hit — return if fresh (< 1 hour).
		if time.Since(entry.fetched) < time.Hour {
			return entry.route
		}
	}

	// Fetch in background to avoid blocking the WebSocket broadcast.
	go l.fetch(callsign)

	// Return cached value (possibly stale) or nil.
	if entry != nil {
		return entry.route
	}
	return nil
}

func (l *Lookup) fetch(callsign string) {
	reqURL := fmt.Sprintf("https://api.adsbdb.com/v0/callsign/%s", url.PathEscape(callsign))
	resp, err := l.client.Get(reqURL)
	if err != nil {
		log.Printf("[routes] fetch error for %s: %v", callsign, err)
		l.cacheResult(callsign, nil)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		l.cacheResult(callsign, nil)
		return
	}

	var result adsbdbResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		l.cacheResult(callsign, nil)
		return
	}

	fr := result.Response.FlightRoute
	if fr.Origin.IATA == "" && fr.Destination.IATA == "" {
		l.cacheResult(callsign, nil)
		return
	}

	route := &Route{
		Origin:      fr.Origin.IATA,
		OriginName:  fr.Origin.Name,
		Destination: fr.Destination.IATA,
		DestName:    fr.Destination.Name,
	}
	l.cacheResult(callsign, route)
}

func (l *Lookup) cacheResult(callsign string, route *Route) {
	l.mu.Lock()
	l.cache[callsign] = &cacheEntry{route: route, fetched: time.Now()}
	l.mu.Unlock()
}

// adsbdb API response types.
type adsbdbResponse struct {
	Response struct {
		FlightRoute struct {
			Origin      adsbdbAirport `json:"origin"`
			Destination adsbdbAirport `json:"destination"`
		} `json:"flightroute"`
	} `json:"response"`
}

type adsbdbAirport struct {
	IATA string `json:"iata_code"`
	ICAO string `json:"icao_code"`
	Name string `json:"name"`
}
