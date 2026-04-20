package uat

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/skytracker/skytracker-device/internal/config"
)

// UATDecoder manages a continuously-running dump978-fa process on a dedicated
// SDR tuned to 978 MHz. Unlike the ACARS decoder, this runs dump978-fa directly
// (no rtl_tcp intermediary, no SatDump). Each stdout line is a JSON object
// representing a decoded UAT frame.
type UATDecoder struct {
	dump978Bin string
	sdrSerial  string
	gain       int
	biasT      bool

	mu      sync.Mutex
	running bool
	frames  chan RawFrame
	uplinks chan RawFrame // raw uplink hex lines from --raw-port
	stats   DecoderStats
	started time.Time
}

// NewUATDecoder creates a decoder for the given config and SDR serial.
func NewUATDecoder(cfg config.UATConfig, sdrSerial string) *UATDecoder {
	bin := cfg.Dump978Bin
	if bin == "" {
		bin = "dump978-fa"
	}
	return &UATDecoder{
		dump978Bin: bin,
		sdrSerial:  sdrSerial,
		gain:       cfg.Gain,
		biasT:      cfg.BiasT,
		frames:     make(chan RawFrame, 1000),
		uplinks:    make(chan RawFrame, 500),
	}
}

// Run is the supervisory loop. It starts the decoder pipeline and restarts
// on failure with exponential backoff. Blocks until ctx is cancelled.
func (d *UATDecoder) Run(ctx context.Context) {
	backoff := 5 * time.Second
	maxBackoff := 60 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		startTime := time.Now()
		err := d.runPipeline(ctx)
		if ctx.Err() != nil {
			return
		}

		runtime := time.Since(startTime)
		if err != nil {
			log.Printf("[uat] pipeline exited after %v: %v", runtime, err)
		} else {
			log.Printf("[uat] pipeline exited after %v", runtime)
		}

		// Reset backoff if the pipeline ran for a reasonable duration.
		if runtime > 60*time.Second {
			backoff = 5 * time.Second
		}

		log.Printf("[uat] restarting in %v", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Exponential backoff.
		backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
	}
}

// Frames returns the read-only channel of decoded raw frames.
func (d *UATDecoder) Frames() <-chan RawFrame {
	return d.frames
}

// UplinkFrames returns raw uplink hex lines from the raw port.
func (d *UATDecoder) UplinkFrames() <-chan RawFrame {
	return d.uplinks
}

// Stats returns a snapshot of decoder statistics.
func (d *UATDecoder) Stats() DecoderStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.stats
	if d.running {
		s.UptimeSeconds = int64(time.Since(d.started).Seconds())
	}
	return s
}

// IsRunning returns whether the decoder pipeline is active.
func (d *UATDecoder) IsRunning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running
}

// runPipeline manages dump978-fa as a separate systemd service and reads
// decoded JSON frames over TCP. dump978-fa block-buffers stdout when spawned
// as a subprocess (C++ iostream default), so we run it independently via
// systemd with --json-port, then connect as a TCP client. This is the
// standard architecture used by readsb/dump1090 stacks.
func (d *UATDecoder) runPipeline(ctx context.Context) error {
	serial := d.sdrSerial
	if serial == "" {
		serial = "0"
	}

	const jsonPort = "30978"
	const rawPort = "30979"
	const serviceName = "dump978-uat"

	// Write the systemd unit file.
	unit := fmt.Sprintf(`[Unit]
Description=UAT 978 MHz decoder (managed by skytracker-agent)

[Service]
ExecStartPre=/bin/bash -c 'for m in rtl2832_sdr dvb_usb_rtl28xxu rtl2832 rtl2830 dvb_usb_v2; do rmmod $m 2>/dev/null; done; true'
ExecStart=%s --sdr driver=rtlsdr,serial=%s --sdr-gain %d --json-port %s --raw-port %s
Restart=always
RestartSec=5
`, d.dump978Bin, serial, d.gain, jsonPort, rawPort)

	if err := os.WriteFile("/etc/systemd/system/"+serviceName+".service", []byte(unit), 0644); err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}

	// Start (or restart) the service.
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "restart", serviceName).Run()
	log.Printf("[uat] started %s service (serial=%s gain=%d port=%s)", serviceName, serial, d.gain, jsonPort)

	// Wait for the TCP port to become available.
	var conn net.Conn
	for i := 0; i < 10; i++ {
		time.Sleep(time.Second)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var err error
		conn, err = net.DialTimeout("tcp", "127.0.0.1:"+jsonPort, 2*time.Second)
		if err == nil {
			break
		}
		log.Printf("[uat] waiting for port %s (%d/10): %v", jsonPort, i+1, err)
	}
	if conn == nil {
		return fmt.Errorf("could not connect to dump978-fa on port %s", jsonPort)
	}
	defer conn.Close()
	log.Printf("[uat] connected to dump978-fa JSON stream on port %s", jsonPort)

	// Connect to the raw port for uplink/FIS-B frames.
	var rawConn net.Conn
	for i := 0; i < 5; i++ {
		var err error
		rawConn, err = net.DialTimeout("tcp", "127.0.0.1:"+rawPort, 2*time.Second)
		if err == nil {
			break
		}
		log.Printf("[uat] waiting for raw port %s (%d/5): %v", rawPort, i+1, err)
		time.Sleep(time.Second)
	}
	if rawConn != nil {
		log.Printf("[uat] connected to dump978-fa raw stream on port %s", rawPort)
		pipelineCtx, pipelineCancel := context.WithCancel(ctx)
		defer pipelineCancel()
		go d.readRawUplinks(pipelineCtx, rawConn)
	} else {
		log.Printf("[uat] warning: could not connect to raw port %s — FIS-B uplinks unavailable", rawPort)
	}

	d.mu.Lock()
	d.running = true
	d.started = time.Now()
	d.stats.FramesDecoded = 0
	d.stats.FrameRate = 0
	d.stats.LastFrameAt = 0
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		d.running = false
		d.mu.Unlock()
	}()

	// Read decoded frames from the TCP connection.
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] != '{' {
			continue
		}

		d.mu.Lock()
		d.stats.FramesDecoded++
		d.stats.LastFrameAt = time.Now().UnixMilli()
		elapsed := time.Since(d.started).Minutes()
		if elapsed > 0 {
			d.stats.FrameRate = float64(d.stats.FramesDecoded) / elapsed
		}
		d.mu.Unlock()

		select {
		case d.frames <- RawFrame{Line: line}:
		default:
			log.Printf("[uat] frame channel full, dropping frame")
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read TCP: %w", err)
	}
	return nil
}



// readRawUplinks reads raw uplink hex lines from the dump978-fa raw port.
// Lines starting with '+' are uplink frames containing FIS-B data.
func (d *UATDecoder) readRawUplinks(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] != '+' {
			continue
		}
		select {
		case d.uplinks <- RawFrame{Line: line}:
		default:
			log.Printf("[uat] uplink channel full, dropping frame")
		}
	}
}

// IncrementFISBStats updates FIS-B product statistics by the given count.
func (d *UATDecoder) IncrementFISBStats(count int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stats.FISBProductsDecoded += count
	d.stats.FISBLastProductAt = time.Now().UnixMilli()
	elapsed := time.Since(d.started).Minutes()
	if elapsed > 0 {
		d.stats.FISBProductRate = float64(d.stats.FISBProductsDecoded) / elapsed
	}
}
