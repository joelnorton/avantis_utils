package detect

import (
	"math"
	"testing"
	"time"
)

// clicks synthesizes decaying 1 kHz bursts at the given spacing.
func clicks(sampleRate int, duration, first, spacing, clickLen time.Duration) []float32 {
	n := int(duration.Seconds() * float64(sampleRate))
	out := make([]float32, n)
	addClicks(out, sampleRate, first, spacing, clickLen)
	return out
}

// addClicks mixes decaying 1 kHz bursts into out.
func addClicks(out []float32, sampleRate int, first, spacing, clickLen time.Duration) {
	n := len(out)
	length := int(clickLen.Seconds() * float64(sampleRate))
	end := time.Duration(int64(n) * int64(time.Second) / int64(sampleRate))

	for onset := first; onset < end; onset += spacing {
		start := int(onset.Seconds() * float64(sampleRate))
		for i := 0; i < length && start+i < n; i++ {
			tt := float64(i) / float64(sampleRate)
			env := math.Exp(-tt / 0.003)
			out[start+i] += float32(0.8 * env * math.Sin(2*math.Pi*1000*tt))
		}
	}
}

func collect(d *Detector, samples []float32, block int) []Event {
	var events []Event
	for i := 0; i < len(samples); i += block {
		end := i + block
		if end > len(samples) {
			end = len(samples)
		}
		if !d.Process(samples[i:end], func(e Event) bool {
			events = append(events, e)
			return true
		}) {
			break
		}
	}
	d.Flush(func(e Event) bool {
		events = append(events, e)
		return true
	})
	return events
}

func TestEnergyPeakIntervals(t *testing.T) {
	const (
		rate    = 44100
		spacing = 500 * time.Millisecond
	)
	cfg := DefaultConfig(rate)
	cfg.PeakMethod = PeakEnergy
	det, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	events := collect(det, clicks(rate, 10*time.Second, 100*time.Millisecond, spacing, 10*time.Millisecond), 137)
	if len(events) != 20 {
		t.Fatalf("got %d peaks, want 20", len(events))
	}
	if events[0].Number != 1 || events[len(events)-1].Number != 20 {
		t.Fatalf("peak numbering wrong: first=%d last=%d", events[0].Number, events[len(events)-1].Number)
	}
	for i := 1; i < len(events); i++ {
		interval := events[i].Time - events[i-1].Time
		if diff := interval - spacing; math.Abs(float64(diff)) > float64(200*time.Microsecond) {
			t.Fatalf("interval %d = %v, want %v (diff %v)", i, interval, spacing, diff)
		}
	}
	if events[0].Level <= 0.5 {
		t.Fatalf("first peak level = %v, want > 0.5", events[0].Level)
	}
}

func TestMaxPeakIntervals(t *testing.T) {
	const (
		rate    = 48000
		spacing = 300 * time.Millisecond
	)
	cfg := DefaultConfig(rate)
	cfg.PeakMethod = PeakMax
	det, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	events := collect(det, clicks(rate, 5*time.Second, 50*time.Millisecond, spacing, 10*time.Millisecond), 512)
	if len(events) != 17 {
		t.Fatalf("got %d peaks, want 17", len(events))
	}
	for i := 1; i < len(events); i++ {
		interval := events[i].Time - events[i-1].Time
		if diff := interval - spacing; math.Abs(float64(diff)) > float64(time.Millisecond) {
			t.Fatalf("interval %d = %v, want %v (diff %v)", i, interval, spacing, diff)
		}
	}
}

func TestRefractoryIgnoresClosePeaks(t *testing.T) {
	const rate = 44100
	cfg := DefaultConfig(rate)
	cfg.MinInterval = 30 * time.Millisecond
	det, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Two clicks 10ms apart: the second falls inside the refractory period.
	sig := clicks(rate, 500*time.Millisecond, 100*time.Millisecond, time.Second, 5*time.Millisecond)
	addClicks(sig, rate, 110*time.Millisecond, time.Second, 5*time.Millisecond)
	events := collect(det, sig, 256)
	if len(events) != 1 {
		t.Fatalf("got %d peaks, want 1", len(events))
	}
}

func TestFlushEmitsPendingPeak(t *testing.T) {
	const rate = 44100
	cfg := DefaultConfig(rate)
	cfg.PeakWindow = 20 * time.Millisecond
	det, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// The click is so close to the end that its search window is truncated.
	sig := clicks(rate, 100*time.Millisecond, 95*time.Millisecond, time.Second, 10*time.Millisecond)
	var events []Event
	det.Process(sig, func(e Event) bool {
		events = append(events, e)
		return true
	})
	if len(events) != 0 {
		t.Fatalf("got %d peaks before flush, want 0", len(events))
	}
	det.Flush(func(e Event) bool {
		events = append(events, e)
		return true
	})
	if len(events) != 1 {
		t.Fatalf("got %d peaks after flush, want 1", len(events))
	}
}

func TestDefaultThresholdFindsQuietClicks(t *testing.T) {
	const rate = 44100
	det, err := New(DefaultConfig(rate))
	if err != nil {
		t.Fatal(err)
	}

	sig := clicks(rate, 2*time.Second, 100*time.Millisecond, 400*time.Millisecond, 10*time.Millisecond)
	// Reduce to a quiet, interface-like level (peak about 0.16, roughly -16 dBFS).
	for i := range sig {
		sig[i] *= 0.2
	}
	events := collect(det, sig, 1000)
	if len(events) != 5 {
		t.Fatalf("got %d peaks, want 5", len(events))
	}
}

func TestNewValidatesConfig(t *testing.T) {
	valid := DefaultConfig(44100)
	tests := map[string]func(Config) Config{
		"no sample rate":  func(c Config) Config { c.SampleRate = 0; return c },
		"zero threshold":  func(c Config) Config { c.Threshold = 0; return c },
		"high threshold":  func(c Config) Config { c.Threshold = 2; return c },
		"bad hysteresis":  func(c Config) Config { c.Hysteresis = 1; return c },
		"no release":      func(c Config) Config { c.Release = 0; return c },
		"no min interval": func(c Config) Config { c.MinInterval = 0; return c },
		"no peak window":  func(c Config) Config { c.PeakWindow = 0; return c },
		"negative pre":    func(c Config) Config { c.PreWindow = -time.Millisecond; return c },
	}
	for name, mutate := range tests {
		if _, err := New(mutate(valid)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParsePeakMethod(t *testing.T) {
	for _, s := range []string{"", "energy", "centroid", "ENERGY"} {
		if m, err := ParsePeakMethod(s); err != nil || m != PeakEnergy {
			t.Errorf("ParsePeakMethod(%q) = %v, %v", s, m, err)
		}
	}
	for _, s := range []string{"max", "PEAK"} {
		if m, err := ParsePeakMethod(s); err != nil || m != PeakMax {
			t.Errorf("ParsePeakMethod(%q) = %v, %v", s, m, err)
		}
	}
	if _, err := ParsePeakMethod("nope"); err == nil {
		t.Error("expected error for unknown method")
	}
}
