package sanitize

import (
	"math"
	"testing"
)

func TestSizeAnomaly_Detected(t *testing.T) {
	// Source with mean=2000, stddev=500, count=10.
	// Content at 4 sigma (2000 + 4*500 = 4000) should be detected.
	stats := SourceStats{
		Mean:   2000,
		StdDev: 500,
		Count:  10,
	}

	detected, finding := SizeAnomaly(4001, stats)
	if !detected {
		t.Fatal("expected anomaly to be detected at 4 sigma")
	}
	if finding == nil {
		t.Fatal("expected non-nil finding")
	}
	if finding.Severity != "medium" {
		t.Errorf("severity = %q, want %q", finding.Severity, "medium")
	}
	if finding.Category != "anomaly" {
		t.Errorf("category = %q, want %q", finding.Category, "anomaly")
	}
	if finding.Pattern != "size-anomaly" {
		t.Errorf("pattern = %q, want %q", finding.Pattern, "size-anomaly")
	}
	if finding.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestSizeAnomaly_Normal(t *testing.T) {
	// Content at 2 sigma (2000 + 2*500 = 3000) should NOT be detected.
	stats := SourceStats{
		Mean:   2000,
		StdDev: 500,
		Count:  10,
	}

	detected, finding := SizeAnomaly(3000, stats)
	if detected {
		t.Error("expected no anomaly at 2 sigma")
	}
	if finding != nil {
		t.Errorf("expected nil finding, got %+v", finding)
	}
}

func TestSizeAnomaly_InsufficientSample(t *testing.T) {
	// Count < 5 should skip anomaly detection regardless of content size.
	stats := SourceStats{
		Mean:   100,
		StdDev: 10,
		Count:  3,
	}

	detected, finding := SizeAnomaly(99999, stats)
	if detected {
		t.Error("expected no anomaly with insufficient sample (count=3)")
	}
	if finding != nil {
		t.Errorf("expected nil finding, got %+v", finding)
	}
}

func TestSizeAnomaly_UniformSizes(t *testing.T) {
	// All pages the same size → StdDev=0. No false anomalies should occur.
	stats := ComputeStats([]int{1000, 1000, 1000, 1000, 1000})
	if stats.StdDev != 0 {
		t.Fatalf("expected StdDev=0 for uniform sizes, got %f", stats.StdDev)
	}

	// Even a page slightly larger than the uniform size should not trigger.
	detected, finding := SizeAnomaly(1001, stats)
	if detected {
		t.Error("expected no anomaly with uniform sizes (StdDev=0)")
	}
	if finding != nil {
		t.Errorf("expected nil finding, got %+v", finding)
	}

	// Even a very large page should not trigger when StdDev=0.
	detected, finding = SizeAnomaly(999999, stats)
	if detected {
		t.Error("expected no anomaly with uniform sizes (StdDev=0), even for large content")
	}
	if finding != nil {
		t.Errorf("expected nil finding, got %+v", finding)
	}
}

func TestSizeAnomaly_ZeroStdDev(t *testing.T) {
	// Explicitly constructed stats with StdDev=0 and sufficient count.
	// Must not produce a false anomaly or divide-by-zero panic.
	stats := SourceStats{
		Mean:   500,
		StdDev: 0,
		Count:  10,
	}

	detected, finding := SizeAnomaly(50000, stats)
	if detected {
		t.Error("expected no anomaly when StdDev=0")
	}
	if finding != nil {
		t.Errorf("expected nil finding, got %+v", finding)
	}
}

func TestSizeAnomaly_ExactlyAtThreshold(t *testing.T) {
	// Content exactly at mean + 3*stddev should NOT be detected (> not >=).
	stats := SourceStats{
		Mean:   1000,
		StdDev: 100,
		Count:  10,
	}
	threshold := int(stats.Mean + 3*stats.StdDev) // 1300

	detected, finding := SizeAnomaly(threshold, stats)
	if detected {
		t.Error("expected no anomaly at exactly the threshold (> not >=)")
	}
	if finding != nil {
		t.Errorf("expected nil finding, got %+v", finding)
	}

	// One char over the threshold should be detected.
	detected, finding = SizeAnomaly(threshold+1, stats)
	if !detected {
		t.Error("expected anomaly just above threshold")
	}
	if finding == nil {
		t.Fatal("expected non-nil finding just above threshold")
	}
}

func TestComputeStats(t *testing.T) {
	t.Run("normal distribution", func(t *testing.T) {
		lengths := []int{100, 200, 300, 400, 500}
		stats := ComputeStats(lengths)

		// Mean = (100+200+300+400+500)/5 = 300
		if stats.Mean != 300 {
			t.Errorf("Mean = %f, want 300", stats.Mean)
		}

		// Population StdDev = sqrt(((−200)²+(−100)²+0²+100²+200²)/5)
		//                   = sqrt((40000+10000+0+10000+40000)/5)
		//                   = sqrt(20000) ≈ 141.42
		expectedStdDev := math.Sqrt(20000)
		if math.Abs(stats.StdDev-expectedStdDev) > 0.01 {
			t.Errorf("StdDev = %f, want %f", stats.StdDev, expectedStdDev)
		}

		if stats.Count != 5 {
			t.Errorf("Count = %f, want 5", stats.Count)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		stats := ComputeStats([]int{})
		if stats.Mean != 0 || stats.StdDev != 0 || stats.Count != 0 {
			t.Errorf("expected zero-value SourceStats for empty input, got %+v", stats)
		}
	})

	t.Run("single element", func(t *testing.T) {
		stats := ComputeStats([]int{42})
		if stats.Mean != 42 {
			t.Errorf("Mean = %f, want 42", stats.Mean)
		}
		if stats.StdDev != 0 {
			t.Errorf("StdDev = %f, want 0 for single element", stats.StdDev)
		}
		if stats.Count != 1 {
			t.Errorf("Count = %f, want 1", stats.Count)
		}
	})

	t.Run("two elements", func(t *testing.T) {
		stats := ComputeStats([]int{10, 20})

		// Mean = 15
		if stats.Mean != 15 {
			t.Errorf("Mean = %f, want 15", stats.Mean)
		}

		// Population StdDev = sqrt(((−5)²+5²)/2) = sqrt(25) = 5
		if stats.StdDev != 5 {
			t.Errorf("StdDev = %f, want 5", stats.StdDev)
		}

		if stats.Count != 2 {
			t.Errorf("Count = %f, want 2", stats.Count)
		}
	})

	t.Run("uniform values", func(t *testing.T) {
		stats := ComputeStats([]int{100, 100, 100})
		if stats.Mean != 100 {
			t.Errorf("Mean = %f, want 100", stats.Mean)
		}
		if stats.StdDev != 0 {
			t.Errorf("StdDev = %f, want 0 for uniform values", stats.StdDev)
		}
		if stats.Count != 3 {
			t.Errorf("Count = %f, want 3", stats.Count)
		}
	})

	t.Run("large values", func(t *testing.T) {
		// Verify no overflow issues with large content lengths.
		lengths := []int{1000000, 2000000, 3000000, 4000000, 5000000}
		stats := ComputeStats(lengths)

		if stats.Mean != 3000000 {
			t.Errorf("Mean = %f, want 3000000", stats.Mean)
		}
		if stats.Count != 5 {
			t.Errorf("Count = %f, want 5", stats.Count)
		}
		// StdDev should be non-zero and finite.
		if stats.StdDev <= 0 || math.IsInf(stats.StdDev, 0) || math.IsNaN(stats.StdDev) {
			t.Errorf("StdDev = %f, expected positive finite value", stats.StdDev)
		}
	})
}
