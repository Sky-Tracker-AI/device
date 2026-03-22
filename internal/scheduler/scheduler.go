package scheduler

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/skytracker/skytracker-device/internal/omni"
	"github.com/skytracker/skytracker-device/internal/sat"
	"github.com/skytracker/skytracker-device/internal/sdr"
)

// PassProvider provides upcoming satellite passes.
type PassProvider interface {
	GetDecodablePasses() []sat.PassPrediction
}

// Scheduler manages SDR time-sharing across satellite passes.
type Scheduler struct {
	mu           sync.RWMutex
	sdrs         []sdr.SDRHandle
	passProvider PassProvider
	clock        Clock
	tasks        []*Task
	sdrTasks     map[string]*Task // SDR ID → active task
	state        string           // "running", "stopped"
	decoderFn    func(string) Decoder
	nextTaskID   int
}

// NewScheduler creates a scheduler with the given SDRs and pass provider.
// decoderFn creates a Decoder for a given signal name; defaults to NoopDecoder.
func NewScheduler(sdrs []sdr.SDRHandle, provider PassProvider, decoderFn func(string) Decoder) *Scheduler {
	if decoderFn == nil {
		decoderFn = func(name string) Decoder {
			return NewNoopDecoder(name)
		}
	}
	return &Scheduler{
		sdrs:         sdrs,
		passProvider: provider,
		clock:        RealClock{},
		sdrTasks:     make(map[string]*Task),
		state:        "stopped",
		decoderFn:    decoderFn,
	}
}

// SetClock sets a custom clock (for testing).
func (s *Scheduler) SetClock(c Clock) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock = c
}

// Run starts the scheduler loop with a 10-second tick.
func (s *Scheduler) Run(ctx context.Context) {
	s.mu.Lock()
	s.state = "running"
	s.mu.Unlock()

	log.Printf("[scheduler] started with %d SDR(s)", len(s.sdrs))

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Do an initial tick immediately.
	s.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			s.stop()
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick performs one scheduling cycle.
func (s *Scheduler) tick(ctx context.Context) {
	now := s.clock.Now()

	// 1. Complete tasks that have passed LOS.
	s.completeFinishedTasks(now)

	// 2. Get upcoming decodable passes.
	passes := s.passProvider.GetDecodablePasses()

	// 3. For each pass starting within 60 seconds, try to assign an SDR.
	for _, pass := range passes {
		if pass.AOS.After(now.Add(60 * time.Second)) {
			continue // Not starting soon enough.
		}
		if pass.LOS.Before(now) {
			continue // Already ended.
		}

		// Check if we already have a task for this pass.
		if s.hasTaskForPass(pass) {
			continue
		}

		// Determine priority.
		priority := classifyPriority(pass)

		// Pick a frequency (use first available).
		if len(pass.Frequencies) == 0 {
			continue
		}
		freqMHz := pass.Frequencies[0]
		freqHz := int64(freqMHz * 1e6)

		// Find an idle SDR with compatible tuner.
		handle := s.findIdleSDR(freqMHz)
		if handle == nil {
			// Try preemption: find a lower-priority active task.
			handle = s.preemptLowerPriority(ctx, priority, freqMHz)
		}
		if handle == nil {
			continue // No SDR available.
		}

		// Create and start task.
		task := s.createTask(pass, priority, freqHz, handle)
		if err := s.startTask(ctx, task); err != nil {
			log.Printf("[scheduler] failed to start task %s: %v", task.ID, err)
			task.State = TaskFailed
		}
	}
}

// completeFinishedTasks stops decoders for tasks past LOS.
func (s *Scheduler) completeFinishedTasks(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, task := range s.tasks {
		if task.State != TaskActive {
			continue
		}
		if now.After(task.LOS) {
			if task.Decoder != nil {
				task.Decoder.Stop()
			}
			task.State = TaskCompleted
			task.CompletedAt = now
			delete(s.sdrTasks, task.SDR.ID())
			log.Printf("[scheduler] completed: %s (%s) on SDR %s", task.SatName, task.ID, task.SDR.ID())
		}
	}
}

// hasTaskForPass checks if a task already exists for this pass.
func (s *Scheduler) hasTaskForPass(pass sat.PassPrediction) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, task := range s.tasks {
		if task.NoradID == pass.NoradID && task.AOS.Equal(pass.AOS) &&
			(task.State == TaskPending || task.State == TaskActive) {
			return true
		}
	}
	return false
}

// findIdleSDR finds an SDR not currently assigned to a task.
func (s *Scheduler) findIdleSDR(freqMHz float64) sdr.SDRHandle {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, handle := range s.sdrs {
		if _, busy := s.sdrTasks[handle.ID()]; !busy {
			if isFreqCompatible(handle.TunerType(), freqMHz) {
				return handle
			}
		}
	}
	return nil
}

// preemptLowerPriority stops a lower-priority task to free its SDR.
func (s *Scheduler) preemptLowerPriority(ctx context.Context, newPriority Priority, freqMHz float64) sdr.SDRHandle {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the lowest priority active task on a compatible SDR.
	var candidate *Task
	for _, task := range s.tasks {
		if task.State != TaskActive {
			continue
		}
		if task.Priority >= newPriority {
			continue
		}
		if !isFreqCompatible(task.SDR.TunerType(), freqMHz) {
			continue
		}
		if candidate == nil || task.Priority < candidate.Priority {
			candidate = task
		}
	}

	if candidate == nil {
		return nil
	}

	// Preempt.
	log.Printf("[scheduler] preempting %s (priority %d) for higher priority %d",
		candidate.SatName, candidate.Priority, newPriority)

	if candidate.Decoder != nil {
		candidate.Decoder.Stop()
	}
	candidate.State = TaskCompleted
	candidate.CompletedAt = s.clock.Now()
	handle := candidate.SDR
	delete(s.sdrTasks, handle.ID())

	return handle
}

// createTask creates a new task for a satellite pass.
func (s *Scheduler) createTask(pass sat.PassPrediction, priority Priority, freqHz int64, handle sdr.SDRHandle) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextTaskID++
	task := &Task{
		ID:           fmt.Sprintf("task-%d", s.nextTaskID),
		NoradID:      pass.NoradID,
		SatName:      pass.Name,
		Priority:     priority,
		FreqHz:       freqHz,
		AOS:          pass.AOS,
		LOS:          pass.LOS,
		MaxElevation: pass.MaxElevation,
		State:        TaskPending,
		SDR:          handle,
		Decoder:      s.decoderFn(pass.Name),
	}
	s.tasks = append(s.tasks, task)
	return task
}

// startTask starts a decoder task.
func (s *Scheduler) startTask(ctx context.Context, task *Task) error {
	err := task.Decoder.Start(ctx, task.SDR, task.FreqHz)
	if err != nil {
		return err
	}

	s.mu.Lock()
	task.State = TaskActive
	task.StartedAt = s.clock.Now()
	s.sdrTasks[task.SDR.ID()] = task
	s.mu.Unlock()

	log.Printf("[scheduler] started: %s (priority=%d, freq=%.3f MHz, maxEl=%.1f) on SDR %s",
		task.SatName, task.Priority, float64(task.FreqHz)/1e6, task.MaxElevation, task.SDR.ID())
	return nil
}

// stop gracefully stops all active tasks.
func (s *Scheduler) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, task := range s.tasks {
		if task.State == TaskActive {
			if task.Decoder != nil {
				task.Decoder.Stop()
			}
			task.State = TaskCompleted
			task.CompletedAt = s.clock.Now()
		}
	}
	s.sdrTasks = make(map[string]*Task)
	s.state = "stopped"
	log.Printf("[scheduler] stopped")
}

// State returns the current scheduler state.
func (s *Scheduler) State() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// ActiveDecoder returns the name of the currently active decoder, or "".
func (s *Scheduler) ActiveDecoder() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, task := range s.sdrTasks {
		if task.State == TaskActive && task.Decoder != nil {
			return task.Decoder.Name()
		}
	}
	return ""
}

// ActiveTasks returns tasks currently being decoded.
func (s *Scheduler) ActiveTasks() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var active []*Task
	for _, task := range s.tasks {
		if task.State == TaskActive {
			active = append(active, task)
		}
	}
	return active
}

// UpcomingTasks returns pending tasks sorted by AOS.
func (s *Scheduler) UpcomingTasks() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var pending []*Task
	for _, task := range s.tasks {
		if task.State == TaskPending {
			pending = append(pending, task)
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].AOS.Before(pending[j].AOS)
	})
	return pending
}

// classifyPriority determines the scheduling priority for a pass.
func classifyPriority(pass sat.PassPrediction) Priority {
	switch pass.Category {
	case omni.CatWeather:
		if pass.Decodable {
			return PriorityHigh
		}
		return PriorityLow
	case omni.CatAmateur, omni.CatCubesat:
		if pass.Decodable {
			return PriorityNormal
		}
		return PriorityLow
	default:
		return PriorityLow
	}
}

// isFreqCompatible checks if a tuner type can handle the given frequency.
// R820T/R820T2: 24-1766 MHz (can't do L-band above 1.7 GHz)
// R828D: 24-1766 MHz + extended L-band mode (can reach ~1.8 GHz for Iridium at 1616 MHz)
func isFreqCompatible(tunerType string, freqMHz float64) bool {
	switch tunerType {
	case "R828D":
		return freqMHz >= 24 && freqMHz <= 1800
	case "R820T", "R820T2":
		return freqMHz >= 24 && freqMHz <= 1766
	default:
		// Unknown tuner — assume basic VHF/UHF range.
		return freqMHz >= 24 && freqMHz <= 1766
	}
}
