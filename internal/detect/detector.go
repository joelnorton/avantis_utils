// Package detect finds the loudest events ("peaks") in an audio stream and
// reports their timing.
package detect

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// PeakMethod selects how the time of a peak is estimated inside its detection
// window.
type PeakMethod int

const (
	// PeakEnergy reports the energy centroid of the samples in the window.
	// It is stable against cycle-to-cycle jitter and is the default.
	PeakEnergy PeakMethod = iota
	// PeakMax reports the absolute loudest sample in the window.
	PeakMax
)

// ParsePeakMethod parses a -peak-method value.
func ParsePeakMethod(s string) (PeakMethod, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "energy", "centroid":
		return PeakEnergy, nil
	case "max", "peak":
		return PeakMax, nil
	default:
		return 0, fmt.Errorf("unknown peak method %q (want energy or max)", s)
	}
}

func (m PeakMethod) String() string {
	if m == PeakMax {
		return "max"
	}
	return "energy"
}

// Event is one detected peak.
type Event struct {
	// Number is the 1-based peak number in the stream.
	Number int
	// Sample is the absolute sample index of the peak.
	Sample int64
	// Time is the peak time since the start of the stream.
	Time time.Duration
	// Level is the absolute peak amplitude in the detection window (0..1 for
	// full-scale audio).
	Level float32
}

// Config controls the detector.
type Config struct {
	// SampleRate of the stream, in Hz. Required.
	SampleRate int

	// Threshold is the envelope level that starts a detection, in full-scale
	// amplitude (0..1). Default 0.1.
	Threshold float32

	// Hysteresis is the fraction of Threshold at which the detector re-arms
	// after a peak. Default 0.5 (i.e. re-arm at -6 dB relative to Threshold).
	Hysteresis float32

	// Release is the envelope follower's release time constant. Default 20ms.
	Release time.Duration

	// MinInterval is the refractory period; peaks closer than this are
	// ignored. Default 30ms.
	MinInterval time.Duration

	// PreWindow is how far before the trigger sample the peak search starts.
	// Default 3ms.
	PreWindow time.Duration

	// PeakWindow is how far after the trigger sample the peak search ends.
	// Default 20ms.
	PeakWindow time.Duration

	// PeakMethod selects energy-centroid or absolute-max timing. The zero value
	// is PeakEnergy.
	PeakMethod PeakMethod
}

// DefaultConfig returns a Config with sensible defaults for sampleRate.
func DefaultConfig(sampleRate int) Config {
	return Config{
		SampleRate:  sampleRate,
		Threshold:   0.1,
		Hysteresis:  0.5,
		Release:     20 * time.Millisecond,
		MinInterval: 30 * time.Millisecond,
		PreWindow:   3 * time.Millisecond,
		PeakWindow:  20 * time.Millisecond,
		PeakMethod:  PeakEnergy,
	}
}

// energyFloor ignores window samples below this fraction of the loudest sample
// when computing the energy centroid. It keeps quiet noise from pulling the
// centroid toward the middle of the window.
const energyFloor = 0.1

// Detector is a streaming peak detector. It is not safe for concurrent use.
type Detector struct {
	sampleRate int
	threshold  float32
	armLevel   float32
	decay      float64
	method     PeakMethod

	pre  int64
	post int64
	gap  int64

	ring    []float32
	ringLen int64

	pos     int64 // absolute index of the next sample to process
	env     float32
	armed   bool
	pending *pendingPeak
	last    int64
	emitted int
}

type pendingPeak struct {
	start int64
	end   int64
}

// New validates cfg and returns a detector.
func New(cfg Config) (*Detector, error) {
	if cfg.SampleRate <= 0 {
		return nil, fmt.Errorf("sample rate must be positive, got %d", cfg.SampleRate)
	}
	if cfg.Threshold <= 0 || cfg.Threshold > 1 {
		return nil, fmt.Errorf("threshold must be in (0, 1], got %g", cfg.Threshold)
	}
	if cfg.Hysteresis <= 0 || cfg.Hysteresis >= 1 {
		return nil, fmt.Errorf("hysteresis must be in (0, 1), got %g", cfg.Hysteresis)
	}
	if cfg.Release <= 0 {
		return nil, fmt.Errorf("release must be positive")
	}
	if cfg.MinInterval <= 0 {
		return nil, fmt.Errorf("min interval must be positive")
	}
	if cfg.PreWindow < 0 {
		return nil, fmt.Errorf("pre window must not be negative")
	}
	if cfg.PeakWindow <= 0 {
		return nil, fmt.Errorf("peak window must be positive")
	}

	pre := durationSamples(cfg.PreWindow, cfg.SampleRate)
	post := durationSamples(cfg.PeakWindow, cfg.SampleRate)
	if post < 1 {
		post = 1
	}
	gap := durationSamples(cfg.MinInterval, cfg.SampleRate)
	if gap < 1 {
		gap = 1
	}

	// One extra slot keeps the last `pre+post+1` samples addressable.
	ringLen := pre + post + 2

	return &Detector{
		sampleRate: cfg.SampleRate,
		threshold:  cfg.Threshold,
		armLevel:   cfg.Threshold * cfg.Hysteresis,
		decay:      math.Exp(-1 / (cfg.Release.Seconds() * float64(cfg.SampleRate))),
		method:     cfg.PeakMethod,
		pre:        pre,
		post:       post,
		gap:        gap,
		ring:       make([]float32, ringLen),
		ringLen:    ringLen,
		last:       -1 << 62,
	}, nil
}

func durationSamples(d time.Duration, sampleRate int) int64 {
	if d <= 0 {
		return 0
	}
	return int64(math.Round(d.Seconds() * float64(sampleRate)))
}

// EmitFunc receives a detected peak. Returning false stops processing.
type EmitFunc func(Event) bool

// Process feeds samples to the detector, calling emit for each completed peak.
// It returns false if emit asked to stop.
func (d *Detector) Process(samples []float32, emit EmitFunc) bool {
	for _, x := range samples {
		d.ring[d.pos%d.ringLen] = x

		// Envelope follower: instant attack, exponential release.
		d.env *= float32(d.decay)
		if a := float32(math.Abs(float64(x))); a > d.env {
			d.env = a
		}

		if p := d.pending; p != nil && d.pos >= p.end {
			if !d.finalize(d.pos, emit) {
				return false
			}
		}

		if d.pending == nil {
			if d.armed && d.env >= d.threshold && d.pos-d.last >= d.gap {
				start := d.pos - d.pre
				if start < 0 {
					start = 0
				}
				d.pending = &pendingPeak{start: start, end: d.pos + d.post}
				d.armed = false
			} else if !d.armed && d.env < d.armLevel {
				d.armed = true
			}
		}

		d.pos++
	}
	return true
}

// Flush completes a peak whose search window extends past the end of the
// stream. It returns false if emit asked to stop.
func (d *Detector) Flush(emit EmitFunc) bool {
	if d.pending == nil || d.pos == 0 {
		return true
	}
	return d.finalize(d.pos-1, emit)
}

func (d *Detector) finalize(upto int64, emit EmitFunc) bool {
	p := d.pending
	d.pending = nil

	end := p.end
	if end > upto {
		end = upto
	}
	if end < p.start {
		end = p.start
	}

	// The search is done in two passes over the window: first find the loudest
	// sample, then (for energy timing) weight the samples relative to it.
	var level float32
	maxSample := p.start
	for i := p.start; i <= end; i++ {
		v := d.ring[i%d.ringLen]
		a := float32(math.Abs(float64(v)))
		if a > level {
			level = a
			maxSample = i
		}
	}

	peak := maxSample
	if d.method == PeakEnergy && level > 0 {
		floor := float64(level) * energyFloor
		floor2 := floor * floor
		var sumW, sumT float64
		for i := p.start; i <= end; i++ {
			v := float64(d.ring[i%d.ringLen])
			w := v*v - floor2
			if w <= 0 {
				continue
			}
			sumW += w
			sumT += w * float64(i)
		}
		if sumW > 0 {
			peak = int64(math.Round(sumT / sumW))
		}
	}

	d.last = peak
	d.emitted++

	return emit(Event{
		Number: d.emitted,
		Sample: peak,
		Time:   d.timeAt(peak),
		Level:  level,
	})
}

func (d *Detector) timeAt(sample int64) time.Duration {
	return time.Duration(float64(sample) / float64(d.sampleRate) * float64(time.Second))
}
