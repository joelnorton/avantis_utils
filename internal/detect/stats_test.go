package detect

import (
	"math"
	"testing"
	"time"
)

func TestSummarize(t *testing.T) {
	intervals := []time.Duration{
		400 * time.Millisecond,
		500 * time.Millisecond,
		600 * time.Millisecond,
	}
	s := Summarize(intervals)

	if s.Peaks != 4 || s.Intervals != 3 {
		t.Fatalf("peaks/intervals = %d/%d, want 4/3", s.Peaks, s.Intervals)
	}
	if s.Mean != 500*time.Millisecond || s.Median != 500*time.Millisecond {
		t.Fatalf("mean/median = %v/%v, want 500ms", s.Mean, s.Median)
	}
	if s.Min != 400*time.Millisecond || s.Max != 600*time.Millisecond {
		t.Fatalf("min/max = %v/%v", s.Min, s.Max)
	}
	wantStd := time.Duration(float64(100*time.Millisecond) * math.Sqrt(2.0/3.0))
	if diff := s.StdDev - wantStd; math.Abs(float64(diff)) > float64(time.Microsecond) {
		t.Fatalf("std dev = %v, want %v", s.StdDev, wantStd)
	}
	if math.Abs(s.BPM-120) > 1e-9 {
		t.Fatalf("bpm = %v, want 120", s.BPM)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	s := Summarize(nil)
	if s.Peaks != 0 || s.Intervals != 0 || s.BPM != 0 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}

func TestSummarizeEvenMedian(t *testing.T) {
	s := Summarize([]time.Duration{300 * time.Millisecond, 400 * time.Millisecond})
	if s.Median != 350*time.Millisecond {
		t.Fatalf("median = %v, want 350ms", s.Median)
	}
}
