package queue

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Sighting is an aircraft sighting record queued for upload.
type Sighting struct {
	ID        int64     `json:"-"`
	StationID string    `json:"station_id"`
	Timestamp time.Time `json:"timestamp"`
	ICAOHex   string    `json:"icao_hex"`
	Callsign  string    `json:"callsign"`
	Type      string    `json:"type"`
	Altitude  int       `json:"altitude"`
	Speed     float64   `json:"speed"`
	Heading   float64   `json:"heading"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	Distance  float64   `json:"distance"`
}

// Queue is an offline-capable SQLite queue that buffers aircraft sighting
// data for later upload to skytracker.ai.
type Queue struct {
	db       *sql.DB
	maxBytes int64

	mu sync.Mutex
}

// New creates a new offline queue. The database is stored at dbPath.
// maxMB is the maximum size of the queue database in megabytes.
func New(dbPath string, maxMB int) (*Queue, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create queue dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open queue db: %w", err)
	}

	// Create the sightings table.
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sightings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			data TEXT NOT NULL,
			created_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &Queue{
		db:       db,
		maxBytes: int64(maxMB) * 1024 * 1024,
	}, nil
}

// Enqueue adds sightings to the queue.
func (q *Queue) Enqueue(sightings []Sighting) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	tx, err := q.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO sightings (data, created_at) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, s := range sightings {
		data, err := json.Marshal(s)
		if err != nil {
			log.Printf("[queue] marshal error: %v", err)
			continue
		}
		if _, err := stmt.Exec(string(data), now); err != nil {
			log.Printf("[queue] insert error: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Enforce size limit inline (avoids unbounded goroutine accumulation).
	q.enforceLimitLocked()

	return nil
}

// Dequeue returns up to limit sightings from the queue (oldest first)
// and removes them.
func (q *Queue) Dequeue(limit int) ([]Sighting, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	rows, err := q.db.Query("SELECT id, data FROM sightings ORDER BY id ASC LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var sightings []Sighting
	var ids []int64

	for rows.Next() {
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			continue
		}

		var s Sighting
		if err := json.Unmarshal([]byte(data), &s); err != nil {
			log.Printf("[queue] unmarshal error: %v", err)
			ids = append(ids, id) // still remove corrupt entries
			continue
		}

		s.ID = id
		sightings = append(sightings, s)
		ids = append(ids, id)
	}

	// Delete dequeued rows in bulk.
	if len(ids) > 0 {
		placeholders := make([]string, len(ids))
		args := make([]interface{}, len(ids))
		for i, id := range ids {
			placeholders[i] = "?"
			args[i] = id
		}
		q.db.Exec("DELETE FROM sightings WHERE id IN ("+strings.Join(placeholders, ",")+
			")", args...)
	}

	return sightings, nil
}

// Count returns the number of queued sightings.
func (q *Queue) Count() int {
	var count int
	q.db.QueryRow("SELECT COUNT(*) FROM sightings").Scan(&count)
	return count
}

// Close closes the queue database.
func (q *Queue) Close() error {
	if q.db != nil {
		return q.db.Close()
	}
	return nil
}

// enforceLimitLocked prunes old entries if the DB exceeds maxBytes.
// Must be called with q.mu held.
func (q *Queue) enforceLimitLocked() {
	// Check database file size.
	var pageCount, pageSize int64
	q.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	q.db.QueryRow("PRAGMA page_size").Scan(&pageSize)

	dbSize := pageCount * pageSize
	if dbSize <= q.maxBytes {
		return
	}

	// Delete oldest 10% of records.
	var total int
	q.db.QueryRow("SELECT COUNT(*) FROM sightings").Scan(&total)
	deleteCount := total / 10
	if deleteCount < 100 {
		deleteCount = 100
	}

	_, err := q.db.Exec(
		"DELETE FROM sightings WHERE id IN (SELECT id FROM sightings ORDER BY id ASC LIMIT ?)",
		deleteCount,
	)
	if err != nil {
		log.Printf("[queue] prune error: %v", err)
	} else {
		log.Printf("[queue] pruned %d oldest entries (db size: %d MB, limit: %d MB)",
			deleteCount, dbSize/(1024*1024), q.maxBytes/(1024*1024))
	}
}
