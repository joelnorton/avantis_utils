//go:build cgo

// Package live captures audio from a system input device (an audio interface,
// USB microphone, PipeWire/PulseAudio monitor, etc.).
package live

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/gen2brain/malgo"
)

// Device is an available capture device.
type Device struct {
	Index       int
	Name        string
	Default     bool
	SampleRates []uint32
}

// Options configures a capture stream.
type Options struct {
	// Device selects a capture device by case-insensitive name substring or by
	// the index shown by Devices. Empty means the system default.
	Device string

	// SampleRate is the requested capture rate in Hz. Zero asks the backend for
	// its native rate.
	SampleRate uint32
}

// Stream is an open capture device.
type Stream struct {
	Rate       int
	DeviceName string

	ctx     *malgo.AllocatedContext
	device  *malgo.Device
	samples chan []float32
	stopped chan struct{}
	dropped atomic.Uint64
}

// Devices lists the available capture devices.
func Devices() ([]Device, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("initializing audio backend: %w", err)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	infos, err := ctx.Devices(malgo.Capture)
	if err != nil {
		return nil, fmt.Errorf("listing capture devices: %w", err)
	}

	devices := make([]Device, 0, len(infos))
	for i := range infos {
		d := Device{
			Index:   i,
			Name:    infos[i].Name(),
			Default: infos[i].IsDefault != 0,
		}
		seen := make(map[uint32]bool)
		for _, f := range infos[i].Formats {
			if f.SampleRate != 0 && !seen[f.SampleRate] {
				seen[f.SampleRate] = true
				d.SampleRates = append(d.SampleRates, f.SampleRate)
			}
		}
		devices = append(devices, d)
	}
	return devices, nil
}

// Open initializes a mono float32 capture stream. The caller must call Close.
func Open(opts Options) (*Stream, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("initializing audio backend: %w", err)
	}

	s := &Stream{
		ctx:     ctx,
		samples: make(chan []float32, 256),
		stopped: make(chan struct{}, 1),
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatF32
	cfg.Capture.Channels = 1
	cfg.SampleRate = opts.SampleRate
	// mmap mode is unreliable with the ALSA/PipeWire plugins used by many
	// audio interfaces.
	cfg.Alsa.NoMMap = 1

	if opts.Device != "" {
		id, name, err := selectDevice(ctx, opts.Device)
		if err != nil {
			s.closeBackend()
			return nil, err
		}
		cfg.Capture.DeviceID = unsafe.Pointer(&id)
		s.DeviceName = name
	}

	callbacks := malgo.DeviceCallbacks{
		Data: func(_, in []byte, _ uint32) {
			if len(in) < 4 {
				return
			}
			n := len(in) / 4
			// The buffer belongs to the audio thread, so copy it before
			// handing it to the processing goroutine.
			buf := make([]float32, n)
			for i := range buf {
				buf[i] = math.Float32frombits(binary.NativeEndian.Uint32(in[i*4:]))
			}
			select {
			case s.samples <- buf:
			default:
				// Processing fell behind; drop this block rather than block
				// the real-time thread.
				s.dropped.Add(1)
			}
		},
		Stop: func() {
			select {
			case s.stopped <- struct{}{}:
			default:
			}
		},
	}

	dev, err := malgo.InitDevice(ctx.Context, cfg, callbacks)
	if err != nil && opts.SampleRate != 0 {
		// The device may refuse the requested rate; let it use its native one.
		cfg.SampleRate = 0
		dev, err = malgo.InitDevice(ctx.Context, cfg, callbacks)
	}
	if err != nil {
		s.closeBackend()
		return nil, fmt.Errorf("opening capture device: %w", err)
	}
	s.device = dev

	rate := int(dev.SampleRate())
	if rate == 0 {
		rate = int(opts.SampleRate)
	}
	if rate == 0 {
		rate = 48000
	}
	s.Rate = rate
	if s.DeviceName == "" {
		infos, err := ctx.Devices(malgo.Capture)
		if err == nil {
			for _, info := range infos {
				if info.IsDefault != 0 {
					s.DeviceName = info.Name()
					break
				}
			}
		}
	}
	if s.DeviceName == "" {
		s.DeviceName = "default input"
	}

	return s, nil
}

// Dropped reports how many audio blocks were dropped because processing could
// not keep up. Any non-zero value means interval timing from that point on may
// be inaccurate.
func (s *Stream) Dropped() uint64 {
	return s.dropped.Load()
}

// Run starts the stream and delivers mono float32 blocks to onSamples until ctx
// is cancelled or the device stops.
func (s *Stream) Run(ctx context.Context, onSamples func([]float32)) error {
	if err := s.device.Start(); err != nil {
		return fmt.Errorf("starting capture device: %w", err)
	}
	defer func() {
		_ = s.device.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.stopped:
			return nil
		case buf := <-s.samples:
			onSamples(buf)
		}
	}
}

// Close releases the device and backend resources.
func (s *Stream) Close() {
	if s.device != nil {
		s.device.Uninit()
		s.device = nil
	}
	s.closeBackend()
}

func (s *Stream) closeBackend() {
	if s.ctx != nil {
		_ = s.ctx.Uninit()
		s.ctx.Free()
		s.ctx = nil
	}
}

func selectDevice(ctx *malgo.AllocatedContext, query string) (malgo.DeviceID, string, error) {
	infos, err := ctx.Devices(malgo.Capture)
	if err != nil {
		return malgo.DeviceID{}, "", fmt.Errorf("listing capture devices: %w", err)
	}

	if idx, err := strconv.Atoi(query); err == nil {
		if idx < 0 || idx >= len(infos) {
			return malgo.DeviceID{}, "", fmt.Errorf("capture device index %d out of range (0-%d)", idx, len(infos)-1)
		}
		return infos[idx].ID, infos[idx].Name(), nil
	}

	var matches []malgo.DeviceInfo
	for _, info := range infos {
		if strings.Contains(strings.ToLower(info.Name()), strings.ToLower(query)) {
			matches = append(matches, info)
		}
	}
	if len(matches) == 0 {
		return malgo.DeviceID{}, "", fmt.Errorf("no capture device matches %q; use -list-devices", query)
	}
	if len(matches) > 1 {
		for _, m := range matches {
			if m.IsDefault != 0 {
				return m.ID, m.Name(), nil
			}
		}
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.Name()
		}
		return malgo.DeviceID{}, "", fmt.Errorf("device %q is ambiguous, matches: %s", query, strings.Join(names, "; "))
	}
	return matches[0].ID, matches[0].Name(), nil
}
