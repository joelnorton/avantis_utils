package detect

import (
	"math"
	"sort"
	"time"
)

// Summary describes the intervals between peaks.
type Summary struct {
	Peaks     int
	Intervals int
	Mean      time.Duration
	Median    time.Duration
	StdDev    time.Duration
	Min       time.Duration
	Max       time.Duration
	BPM       float64
}

// Summarize computes statistics for a list of peak-to-peak intervals.
func Summarize(intervals []time.Duration) Summary {
	s := Summary{
		Intervals: len(intervals),
	}
	if len(intervals) > 0 {
		s.Peaks = len(intervals) + 1
	}
	if len(intervals) == 0 {
		return s
	}

	sorted := append([]time.Duration(nil), intervals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	s.Min = sorted[0]
	s.Max = sorted[len(sorted)-1]
	s.Median = percentile(sorted, 0.5)

	var sum float64
	for _, d := range intervals {
		sum += float64(d)
	}
	mean := sum / float64(len(intervals))
	s.Mean = time.Duration(mean)
	if mean > 0 {
		s.BPM = 60_000 / (mean / float64(time.Millisecond))
	}

	var variance float64
	for _, d := range intervals {
		diff := float64(d) - mean
		variance += diff * diff
	}
	s.StdDev = time.Duration(math.Sqrt(variance / float64(len(intervals))))

	return s
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return time.Duration(float64(sorted[lo]) + (float64(sorted[hi])-float64(sorted[lo]))*frac)
}
