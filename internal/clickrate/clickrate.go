// Package clickrate exposes the click-tempo detection used by the web app as
// a small, importable API.
package clickrate

import (
	"context"
	"fmt"
	"time"

	"github.com/joelnorton/avantis_utils/internal/detect"
	"github.com/joelnorton/avantis_utils/internal/live"
)

// Device is an available audio capture device.
type Device struct {
	Index   int
	Name    string
	Default bool
}

// Devices lists the available capture devices.
func Devices() ([]Device, error) {
	raw, err := live.Devices()
	if err != nil {
		return nil, err
	}
	out := make([]Device, len(raw))
	for i, d := range raw {
		out[i] = Device{Index: d.Index, Name: d.Name, Default: d.Default}
	}
	return out, nil
}

// Config controls a measurement run.
type Config struct {
	// Device selects a capture device by name substring or index. Empty means
	// the system default input.
	Device string

	// Duration is how long to listen. Defaults to 5 seconds.
	Duration time.Duration

	// Threshold is the trigger level in full-scale amplitude (0..1). Default 0.1.
	Threshold float32

	// MinInterval ignores peaks closer together than this. Default 30ms.
	MinInterval time.Duration
}

// Result is the outcome of a measurement.
type Result struct {
	// Intervals in milliseconds between consecutive peaks.
	Intervals []float64
	// MeanMS is the mean peak-to-peak interval in milliseconds.
	MeanMS float64
	// MedianMS is the median peak-to-peak interval in milliseconds.
	MedianMS float64
	// Peaks is the number of detected peaks.
	Peaks int
	// PeaksPerSecond is the detected rate in peaks per second.
	PeaksPerSecond float64
	// BPM is the detected tempo.
	BPM float64
	// DeviceName is the resolved capture device name.
	DeviceName string
	// SampleRate is the negotiated capture rate.
	SampleRate int
}

// Measure listens to the selected device for cfg.Duration and returns the peak
// rate statistics. The context can be used to stop early (e.g. Ctrl+C).
func Measure(ctx context.Context, cfg Config) (*Result, error) {
	if cfg.Duration <= 0 {
		cfg.Duration = 5 * time.Second
	}
	if cfg.Threshold <= 0 {
		cfg.Threshold = 0.1
	}
	if cfg.MinInterval <= 0 {
		cfg.MinInterval = 30 * time.Millisecond
	}

	stream, err := live.Open(live.Options{Device: cfg.Device, SampleRate: 48000})
	if err != nil {
		return nil, fmt.Errorf("opening capture device: %w", err)
	}
	defer stream.Close()

	detCfg := detect.DefaultConfig(stream.Rate)
	detCfg.Threshold = cfg.Threshold
	detCfg.MinInterval = cfg.MinInterval
	det, err := detect.New(detCfg)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()

	var events []detect.Event
	emit := func(e detect.Event) bool {
		events = append(events, e)
		return true
	}

	if err := stream.Run(runCtx, func(buf []float32) {
		det.Process(buf, emit)
	}); err != nil {
		return nil, err
	}
	det.Flush(emit)

	if len(events) < 2 {
		return nil, fmt.Errorf("only %d peak(s) detected in %s; raise the input level or lower the threshold",
			len(events), cfg.Duration)
	}

	intervals := make([]time.Duration, 0, len(events)-1)
	msList := make([]float64, 0, len(events)-1)
	for i := 1; i < len(events); i++ {
		d := events[i].Time - events[i-1].Time
		intervals = append(intervals, d)
		msList = append(msList, float64(d)/float64(time.Millisecond))
	}

	s := detect.Summarize(intervals)

	return &Result{
		Intervals:      msList,
		MeanMS:         float64(s.Mean) / float64(time.Millisecond),
		MedianMS:       float64(s.Median) / float64(time.Millisecond),
		Peaks:          len(events),
		PeaksPerSecond: float64(len(events)) / cfg.Duration.Seconds(),
		BPM:            s.BPM,
		DeviceName:     stream.DeviceName,
		SampleRate:     stream.Rate,
	}, nil
}
