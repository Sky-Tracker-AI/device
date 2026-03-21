package platform

import (
	"context"
	"log"
	"sync"
	"time"
)

// RegistrationRetrier attempts device registration with exponential backoff.
// It is safe to call AttemptRegistration on every health tick; the retrier
// internally tracks whether backoff has elapsed before making another attempt.
type RegistrationRetrier struct {
	mu          sync.Mutex
	backoff     time.Duration
	lastAttempt time.Time
	registered  bool

	minBackoff time.Duration
	maxBackoff time.Duration
}

// NewRegistrationRetrier creates a retrier with backoff starting at 30s
// and capping at 30 minutes.
func NewRegistrationRetrier() *RegistrationRetrier {
	return &RegistrationRetrier{
		minBackoff: 30 * time.Second,
		maxBackoff: 30 * time.Minute,
		backoff:    30 * time.Second,
	}
}

// IsRegistered returns true if a previous attempt succeeded.
func (r *RegistrationRetrier) IsRegistered() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registered
}

// MarkRegistered marks registration as complete (e.g. if it was already
// registered before the retrier was created).
func (r *RegistrationRetrier) MarkRegistered() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registered = true
}

// AttemptRegistration calls registerFn if enough backoff time has elapsed
// since the last failed attempt. On success it marks the device as registered
// and returns the response. On failure it doubles the backoff (up to the cap)
// and returns nil. If registration is already complete or backoff hasn't
// elapsed, it returns nil without calling registerFn.
func (r *RegistrationRetrier) AttemptRegistration(ctx context.Context, registerFn func(ctx context.Context) (*RegisterResponse, error)) *RegisterResponse {
	r.mu.Lock()
	if r.registered {
		r.mu.Unlock()
		return nil
	}
	if !r.lastAttempt.IsZero() && time.Since(r.lastAttempt) < r.backoff {
		r.mu.Unlock()
		return nil
	}
	r.lastAttempt = time.Now()
	r.mu.Unlock()

	resp, err := registerFn(ctx)
	if err != nil {
		r.mu.Lock()
		log.Printf("[platform] registration retry failed (next in %v): %v", r.backoff*2, err)
		r.backoff *= 2
		if r.backoff > r.maxBackoff {
			r.backoff = r.maxBackoff
		}
		r.mu.Unlock()
		return nil
	}

	r.mu.Lock()
	r.registered = true
	r.mu.Unlock()
	return resp
}
