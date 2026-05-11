package sanitize

import (
	"fmt"
	"math"
)

// ComputeStats computes content size statistics from a slice of document
// lengths. Returns a SourceStats with the mean, standard deviation, and
// count of the input.
//
// Edge cases:
//   - Empty slice: returns zero-value SourceStats (Mean=0, StdDev=0, Count=0).
//   - Single element: returns StdDev=0 (no variance with one sample).
//
// The standard deviation uses the population formula (dividing by N, not
// N-1) because we are computing stats over the entire source batch, not
// estimating from a sample.
func ComputeStats(lengths []int) SourceStats {
	n := len(lengths)
	if n == 0 {
		return SourceStats{}
	}

	// Compute mean.
	var sum float64
	for _, l := range lengths {
		sum += float64(l)
	}
	mean := sum / float64(n)

	// Compute population standard deviation.
	if n == 1 {
		return SourceStats{
			Mean:   mean,
			StdDev: 0,
			Count:  1,
		}
	}

	var varianceSum float64
	for _, l := range lengths {
		diff := float64(l) - mean
		varianceSum += diff * diff
	}
	stddev := math.Sqrt(varianceSum / float64(n))

	return SourceStats{
		Mean:   mean,
		StdDev: stddev,
		Count:  float64(n),
	}
}

// SizeAnomaly detects content whose size deviates significantly from the
// source's average page size. Returns true with a finding when contentLen
// exceeds mean + 3*stddev and the sample size is at least 5.
//
// Returns false, nil when:
//   - stats.Count < 5 (insufficient sample for meaningful statistics)
//   - stats.StdDev == 0 (all pages are the same size; no outlier possible)
//   - contentLen is within 3 standard deviations of the mean
//
// Design decision: The 3-sigma threshold balances sensitivity with false
// positive rate. For normally distributed page sizes, only ~0.3% of pages
// would exceed this threshold by chance. The Count >= 5 guard prevents
// unreliable statistics from small batches (per FR-SAN-005).
func SizeAnomaly(contentLen int, stats SourceStats) (bool, *Finding) {
	// Insufficient sample: need at least 5 documents for meaningful stats.
	if stats.Count < 5 {
		return false, nil
	}

	// Zero standard deviation: all pages are the same size. No outlier
	// is possible, and dividing by zero must be avoided.
	if stats.StdDev == 0 {
		return false, nil
	}

	threshold := stats.Mean + 3*stats.StdDev
	size := float64(contentLen)

	if size > threshold {
		deviation := (size - stats.Mean) / stats.StdDev
		return true, &Finding{
			Pattern:  "size-anomaly",
			Severity: "medium",
			Category: "anomaly",
			Message: fmt.Sprintf(
				"page size %d chars is %.1fx standard deviations from source mean %.0f (stddev %.0f)",
				contentLen, deviation, stats.Mean, stats.StdDev,
			),
		}
	}

	return false, nil
}
