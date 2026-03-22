package scheduler

import (
	"context"
	"time"

	"github.com/skytracker/skytracker-device/internal/sdr"
)

// Priority levels for scheduling decoder tasks.
type Priority int

const (
	PriorityHigh   Priority = 3 // Weather satellite passes (NOAA/Meteor APT/LRPT)
	PriorityNormal Priority = 2 // Amateur/CubeSat decodable passes
	PriorityLow    Priority = 1 // Non-decodable observation passes
)

// TaskState tracks the lifecycle of a decoder task.
type TaskState string

const (
	TaskPending   TaskState = "pending"
	TaskActive    TaskState = "active"
	TaskCompleted TaskState = "completed"
	TaskFailed    TaskState = "failed"
)

// Decoder is the interface for signal decoders. Phase 1 uses NoopDecoder.
type Decoder interface {
	Name() string
	Start(ctx context.Context, sdr sdr.SDRHandle, freqHz int64) error
	Stop() error
	IsRunning() bool
}

// Task represents a scheduled decoder task for a satellite pass.
type Task struct {
	ID           string
	NoradID      int
	SatName      string
	Priority     Priority
	FreqHz       int64
	AOS          time.Time
	LOS          time.Time
	MaxElevation float64
	State        TaskState
	SDR          sdr.SDRHandle
	Decoder      Decoder
	StartedAt    time.Time
	CompletedAt  time.Time
}

// Clock interface for deterministic time control in tests.
type Clock interface {
	Now() time.Time
}

// RealClock uses the system clock.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// MockClock allows setting time explicitly for testing.
type MockClock struct {
	T time.Time
}

func (m *MockClock) Now() time.Time    { return m.T }
func (m *MockClock) Set(t time.Time)   { m.T = t }
func (m *MockClock) Advance(d time.Duration) { m.T = m.T.Add(d) }
