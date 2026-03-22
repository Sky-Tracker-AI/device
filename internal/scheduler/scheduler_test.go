package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/skytracker/skytracker-device/internal/omni"
	"github.com/skytracker/skytracker-device/internal/sat"
	"github.com/skytracker/skytracker-device/internal/sdr"
)

// mockPassProvider implements PassProvider for testing.
type mockPassProvider struct {
	passes []sat.PassPrediction
}

func (m *mockPassProvider) GetDecodablePasses() []sat.PassPrediction {
	return m.passes
}

func newMockSDR(id, serial, tuner string) sdr.SDRHandle {
	return &sdr.MockSDRHandle{
		MockID:     id,
		MockSerial: serial,
		MockTuner:  tuner,
	}
}

func TestSchedulerBasicTask(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := &MockClock{T: now}

	mockSDR := newMockSDR("sdr-0", "SKT-OMNI-0", "R820T")
	provider := &mockPassProvider{
		passes: []sat.PassPrediction{
			{
				NoradID:      25338,
				Name:         "NOAA 15",
				Category:     omni.CatWeather,
				AOS:          now.Add(30 * time.Second),
				LOS:          now.Add(15 * time.Minute),
				MaxElevation: 45.0,
				Decodable:    true,
				Frequencies:  []float64{137.62},
			},
		},
	}

	sched := NewScheduler([]sdr.SDRHandle{mockSDR}, provider, nil)
	sched.SetClock(clock)

	ctx := context.Background()
	sched.tick(ctx)

	active := sched.ActiveTasks()
	if len(active) != 1 {
		t.Fatalf("expected 1 active task, got %d", len(active))
	}

	task := active[0]
	if task.SatName != "NOAA 15" {
		t.Errorf("task name = %q, want NOAA 15", task.SatName)
	}
	if task.Priority != PriorityHigh {
		t.Errorf("task priority = %d, want %d (high)", task.Priority, PriorityHigh)
	}
	if task.FreqHz != 137620000 {
		t.Errorf("task freq = %d, want 137620000", task.FreqHz)
	}
}

func TestSchedulerCompletesAtLOS(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := &MockClock{T: now}

	mockSDR := newMockSDR("sdr-0", "SKT-OMNI-0", "R820T")
	provider := &mockPassProvider{
		passes: []sat.PassPrediction{
			{
				NoradID:      25338,
				Name:         "NOAA 15",
				Category:     omni.CatWeather,
				AOS:          now.Add(10 * time.Second),
				LOS:          now.Add(5 * time.Minute),
				MaxElevation: 30.0,
				Decodable:    true,
				Frequencies:  []float64{137.62},
			},
		},
	}

	sched := NewScheduler([]sdr.SDRHandle{mockSDR}, provider, nil)
	sched.SetClock(clock)

	ctx := context.Background()
	sched.tick(ctx)

	if len(sched.ActiveTasks()) != 1 {
		t.Fatal("expected 1 active task after first tick")
	}

	// Advance past LOS.
	clock.Advance(6 * time.Minute)
	provider.passes = nil // Pass is over.
	sched.tick(ctx)

	if len(sched.ActiveTasks()) != 0 {
		t.Error("expected 0 active tasks after LOS")
	}
}

func TestSchedulerPreemption(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := &MockClock{T: now}

	mockSDR := newMockSDR("sdr-0", "SKT-OMNI-0", "R820T")

	// Start with a low-priority pass.
	provider := &mockPassProvider{
		passes: []sat.PassPrediction{
			{
				NoradID:      44909,
				Name:         "TEVEL-1",
				Category:     omni.CatAmateur,
				AOS:          now.Add(10 * time.Second),
				LOS:          now.Add(10 * time.Minute),
				MaxElevation: 20.0,
				Decodable:    true,
				Frequencies:  []float64{436.4},
			},
		},
	}

	sched := NewScheduler([]sdr.SDRHandle{mockSDR}, provider, nil)
	sched.SetClock(clock)

	ctx := context.Background()
	sched.tick(ctx)

	active := sched.ActiveTasks()
	if len(active) != 1 || active[0].SatName != "TEVEL-1" {
		t.Fatal("expected TEVEL-1 to be active")
	}

	// Now a high-priority weather pass arrives.
	clock.Advance(30 * time.Second)
	provider.passes = append(provider.passes, sat.PassPrediction{
		NoradID:      25338,
		Name:         "NOAA 15",
		Category:     omni.CatWeather,
		AOS:          clock.Now().Add(10 * time.Second),
		LOS:          clock.Now().Add(12 * time.Minute),
		MaxElevation: 60.0,
		Decodable:    true,
		Frequencies:  []float64{137.62},
	})

	sched.tick(ctx)

	active = sched.ActiveTasks()
	if len(active) != 1 {
		t.Fatalf("expected 1 active task after preemption, got %d", len(active))
	}
	if active[0].SatName != "NOAA 15" {
		t.Errorf("expected NOAA 15 active, got %s", active[0].SatName)
	}
}

func TestSchedulerMultiSDR(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := &MockClock{T: now}

	sdr0 := newMockSDR("sdr-0", "SKT-OMNI-0", "R820T")
	sdr1 := newMockSDR("sdr-1", "SKT-OMNI-1", "R820T")

	provider := &mockPassProvider{
		passes: []sat.PassPrediction{
			{
				NoradID:      25338,
				Name:         "NOAA 15",
				Category:     omni.CatWeather,
				AOS:          now.Add(10 * time.Second),
				LOS:          now.Add(10 * time.Minute),
				MaxElevation: 45.0,
				Decodable:    true,
				Frequencies:  []float64{137.62},
			},
			{
				NoradID:      33591,
				Name:         "NOAA 19",
				Category:     omni.CatWeather,
				AOS:          now.Add(20 * time.Second),
				LOS:          now.Add(12 * time.Minute),
				MaxElevation: 30.0,
				Decodable:    true,
				Frequencies:  []float64{137.1},
			},
		},
	}

	sched := NewScheduler([]sdr.SDRHandle{sdr0, sdr1}, provider, nil)
	sched.SetClock(clock)

	ctx := context.Background()
	sched.tick(ctx)

	active := sched.ActiveTasks()
	if len(active) != 2 {
		t.Fatalf("expected 2 active tasks with 2 SDRs, got %d", len(active))
	}
}

func TestSchedulerFrequencyCompatibility(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := &MockClock{T: now}

	// Only an R820T SDR — can't do Iridium at 1616 MHz (above 1766 is fine, but let's test L-band).
	mockSDR := newMockSDR("sdr-0", "SKT-OMNI-0", "R820T")

	provider := &mockPassProvider{
		passes: []sat.PassPrediction{
			{
				NoradID:     41187,
				Name:        "IRIDIUM 106",
				Category:    omni.CatComm,
				AOS:         now.Add(10 * time.Second),
				LOS:         now.Add(5 * time.Minute),
				Decodable:   true,
				Frequencies: []float64{1616.0}, // Within R820T range (24-1766 MHz)
			},
		},
	}

	sched := NewScheduler([]sdr.SDRHandle{mockSDR}, provider, nil)
	sched.SetClock(clock)

	ctx := context.Background()
	sched.tick(ctx)

	// 1616 MHz is within R820T range, so it should work.
	active := sched.ActiveTasks()
	if len(active) != 1 {
		t.Fatalf("expected 1 active task (1616 MHz in R820T range), got %d", len(active))
	}
}

func TestClassifyPriority(t *testing.T) {
	tests := []struct {
		name     string
		pass     sat.PassPrediction
		expected Priority
	}{
		{
			name:     "weather decodable",
			pass:     sat.PassPrediction{Category: omni.CatWeather, Decodable: true},
			expected: PriorityHigh,
		},
		{
			name:     "weather non-decodable",
			pass:     sat.PassPrediction{Category: omni.CatWeather, Decodable: false},
			expected: PriorityLow,
		},
		{
			name:     "amateur decodable",
			pass:     sat.PassPrediction{Category: omni.CatAmateur, Decodable: true},
			expected: PriorityNormal,
		},
		{
			name:     "cubesat decodable",
			pass:     sat.PassPrediction{Category: omni.CatCubesat, Decodable: true},
			expected: PriorityNormal,
		},
		{
			name:     "science non-decodable",
			pass:     sat.PassPrediction{Category: omni.CatScience, Decodable: false},
			expected: PriorityLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyPriority(tt.pass)
			if got != tt.expected {
				t.Errorf("classifyPriority = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestIsFreqCompatible(t *testing.T) {
	tests := []struct {
		tuner   string
		freqMHz float64
		want    bool
	}{
		{"R820T", 137.1, true},
		{"R820T", 435.0, true},
		{"R820T", 1616.0, true},
		{"R820T", 1800.0, false},
		{"R828D", 1616.0, true},
		{"R828D", 1800.0, true},
		{"R828D", 1900.0, false},
		{"unknown", 137.1, true},
		{"unknown", 10.0, false},
	}

	for _, tt := range tests {
		got := isFreqCompatible(tt.tuner, tt.freqMHz)
		if got != tt.want {
			t.Errorf("isFreqCompatible(%q, %.1f) = %v, want %v", tt.tuner, tt.freqMHz, got, tt.want)
		}
	}
}

func TestSchedulerState(t *testing.T) {
	sdr0 := newMockSDR("sdr-0", "SKT-OMNI-0", "R820T")
	provider := &mockPassProvider{}

	sched := NewScheduler([]sdr.SDRHandle{sdr0}, provider, nil)
	if sched.State() != "stopped" {
		t.Errorf("initial state = %q, want stopped", sched.State())
	}

	ctx, cancel := context.WithCancel(context.Background())
	go sched.Run(ctx)

	// Wait briefly for goroutine to start.
	time.Sleep(50 * time.Millisecond)
	if sched.State() != "running" {
		t.Errorf("running state = %q, want running", sched.State())
	}

	cancel()
	time.Sleep(50 * time.Millisecond)
	if sched.State() != "stopped" {
		t.Errorf("stopped state = %q, want stopped", sched.State())
	}
}

func TestNoopDecoder(t *testing.T) {
	d := NewNoopDecoder("test-signal")
	if d.Name() != "test-signal" {
		t.Errorf("name = %q, want test-signal", d.Name())
	}
	if d.IsRunning() {
		t.Error("expected not running initially")
	}

	handle := newMockSDR("sdr-0", "SKT-OMNI-0", "R820T")
	err := d.Start(context.Background(), handle, 137100000)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !d.IsRunning() {
		t.Error("expected running after Start")
	}

	err = d.Stop()
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if d.IsRunning() {
		t.Error("expected not running after Stop")
	}
}
