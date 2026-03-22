package scheduler

import (
	"context"
	"log"
	"sync"

	"github.com/skytracker/skytracker-device/internal/sdr"
)

// NoopDecoder logs decoder actions without performing any real signal processing.
// Used in Phase 1 to test the scheduler framework.
type NoopDecoder struct {
	name    string
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
}

// NewNoopDecoder creates a NoopDecoder with the given signal name.
func NewNoopDecoder(name string) *NoopDecoder {
	return &NoopDecoder{name: name}
}

func (d *NoopDecoder) Name() string {
	return d.name
}

func (d *NoopDecoder) Start(ctx context.Context, handle sdr.SDRHandle, freqHz int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	log.Printf("[noop-decoder] would start %q on SDR %s at %.3f MHz",
		d.name, handle.ID(), float64(freqHz)/1e6)

	d.running = true
	// Store cancel func so Stop() can release the derived context.
	// Real decoders will pass the child context to their processing goroutine.
	var childCtx context.Context
	childCtx, d.cancel = context.WithCancel(ctx)
	_ = childCtx // NoopDecoder doesn't use it; real decoders will.
	return nil
}

func (d *NoopDecoder) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	log.Printf("[noop-decoder] would stop %q", d.name)

	d.running = false
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	return nil
}

func (d *NoopDecoder) IsRunning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running
}
