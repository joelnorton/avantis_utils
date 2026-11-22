//go:build !cgo

// Package live provides live audio capture when the binary is built with cgo.
package live

import (
	"context"
	"errors"
)

// ErrUnavailable is returned when live capture support was not compiled in.
var ErrUnavailable = errors.New("live audio capture requires cgo; rebuild with CGO_ENABLED=1 (WAV file mode works without it)")

// Device is an available capture device.
type Device struct {
	Index       int
	Name        string
	Default     bool
	SampleRates []uint32
}

// Options configures a capture stream.
type Options struct {
	Device     string
	SampleRate uint32
}

// Stream is an open capture device.
type Stream struct {
	Rate       int
	DeviceName string
}

// Devices is unavailable without cgo.
func Devices() ([]Device, error) { return nil, ErrUnavailable }

// Open is unavailable without cgo.
func Open(Options) (*Stream, error) { return nil, ErrUnavailable }

// Run is unavailable without cgo.
func (s *Stream) Run(context.Context, func([]float32)) error { return ErrUnavailable }

// Close releases resources.
func (s *Stream) Close() {}

// Dropped reports dropped blocks (always zero without cgo).
func (s *Stream) Dropped() uint64 { return 0 }
