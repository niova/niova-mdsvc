package clictlplanefuncs

// placement_stats_test.go
//
// Pure, cluster-independent statistical primitives for evaluating the fairness
// of any chunk-placement algorithm. Nothing in this file talks to the control
// plane; it operates on plain []float64 sample sets (typically "number of
// placements landing on each entity at one hierarchy level"). Because it is
// pure, it is exercised directly by TestPlacementStats below and reused by the
// integration suite (see placement_distribution_test.go).
//
// The same DistStats/FairnessThresholds vocabulary is intended to outlive the
// current EC distribution algorithm: any future placement algorithm can be held
// to the same statistical contract simply by feeding it different sample sets.

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// DistStats is the full statistical summary of one sample set. Every field is
// derivable from the Chunk Placement Matrix, so the same struct describes the
// spread of placements across PDUs, racks, hypervisors, devices, or NISDs, for
// data-only, parity-only, or combined counts.
type DistStats struct {
	Count       int     // number of eligible entities (samples)
	Sum         float64 // total placements observed
	Mean        float64 // Sum / Count
	Min         float64 // smallest sample
	Max         float64 // largest sample
	Range       float64 // Max - Min
	Variance    float64 // population variance
	StdDev      float64 // population standard deviation
	CV          float64 // coefficient of variation: StdDev / Mean (scale-free)
	Jain        float64 // Jain's fairness index in [1/Count, 1]; 1 == perfectly fair
	MaxMinRatio float64 // Max / Min (1 when perfectly even; +Inf if some entity is empty)
	Skew        float64 // population Fisher-Pearson skewness; 0 == symmetric
}

// ComputeStats reduces a sample set to a DistStats. Degenerate inputs are
// handled so callers never divide by zero:
//   - empty input      -> zero value
//   - all-equal input  -> Variance/StdDev/CV/Skew 0, Jain 1, MaxMinRatio 1
//   - all-zero input   -> Jain 1 (vacuously fair), MaxMinRatio 1
func ComputeStats(values []float64) DistStats {
	var s DistStats
	s.Count = len(values)
	if s.Count == 0 {
		return s
	}

	s.Min, s.Max = values[0], values[0]
	for _, v := range values {
		s.Sum += v
		if v < s.Min {
			s.Min = v
		}
		if v > s.Max {
			s.Max = v
		}
	}
	s.Mean = s.Sum / float64(s.Count)
	s.Range = s.Max - s.Min

	var sqSum, cubeSum, sqValSum float64
	for _, v := range values {
		d := v - s.Mean
		sqSum += d * d
		cubeSum += d * d * d
		sqValSum += v * v
	}
	s.Variance = sqSum / float64(s.Count)
	s.StdDev = math.Sqrt(s.Variance)

	if s.Mean != 0 {
		s.CV = s.StdDev / s.Mean
	}

	switch {
	case s.Min != 0:
		s.MaxMinRatio = s.Max / s.Min
	case s.Max == 0: // all zero
		s.MaxMinRatio = 1
	default: // some entity got nothing while others did
		s.MaxMinRatio = math.Inf(1)
	}

	// Jain's fairness index: (Σx)² / (n·Σx²). Independent of scale and metric.
	if sqValSum != 0 {
		s.Jain = (s.Sum * s.Sum) / (float64(s.Count) * sqValSum)
	} else {
		s.Jain = 1 // nothing placed anywhere -> vacuously fair
	}

	if s.StdDev != 0 {
		s.Skew = (cubeSum / float64(s.Count)) / (s.StdDev * s.StdDev * s.StdDev)
	}
	return s
}

// FairnessThresholds expresses the acceptance criteria a distribution must meet.
// Each bound has a documented rationale (see the named constructors). A zero or
// "unset" bound means "do not check this dimension".
type FairnessThresholds struct {
	MinJain     float64 // lower bound on Jain index (0 disables)
	MaxCV       float64 // upper bound on coefficient of variation (0 disables)
	MaxMaxMin   float64 // upper bound on Max/Min ratio (0 disables)
	MaxAbsSkew  float64 // upper bound on |skew| (0 disables)
	MinSamples  int     // below this entity count, fairness is not asserted
	Description string  // human label for reporting
}

// EqualSizedThresholds is the strict bar applied when every eligible entity has
// identical capacity, so a fair algorithm should distribute almost perfectly.
//
// Rationale:
//   - Jain >= 0.95: with N equal entities a single fully-idle entity out of ~20
//     already pulls Jain below 0.95, so 0.95 catches "one box never chosen".
//   - CV <= 0.10: at most ~10% relative spread around the mean.
//   - Max/Min <= 1.50: the busiest entity holds at most 1.5x the quietest; with
//     small counts an off-by-one between entities is unavoidable, so we do not
//     demand 1.0.
//   - |skew| <= 0.75: rules out a long one-sided tail (a few hot entities).
func EqualSizedThresholds() FairnessThresholds {
	return FairnessThresholds{
		MinJain: 0.95, MaxCV: 0.10, MaxMaxMin: 1.50, MaxAbsSkew: 0.75,
		MinSamples: 2, Description: "equal-sized (strict)",
	}
}

// GeneralThresholds is the relaxed bar used when topology is unbalanced or the
// sample counts are small, where perfect evenness is not achievable but gross
// bias must still be caught.
//
// Rationale: Jain >= 0.85 and CV <= 0.25 tolerate the lumpiness that an
// unbalanced hierarchy forces, while still failing if, say, one rack soaks up
// twice its fair share.
func GeneralThresholds() FairnessThresholds {
	return FairnessThresholds{
		MinJain: 0.85, MaxCV: 0.25, MaxMaxMin: 2.0, MaxAbsSkew: 1.5,
		MinSamples: 2, Description: "general (relaxed)",
	}
}

// Check evaluates a DistStats against the thresholds. It returns ok plus a list
// of human-readable reasons for any failures. Checks are skipped (counted as
// passing) when there are too few samples or nothing was placed, so callers can
// apply one threshold set uniformly across levels of differing cardinality.
func (s DistStats) Check(thr FairnessThresholds) (ok bool, reasons []string) {
	if s.Count < max2(thr.MinSamples, 1) || s.Sum == 0 {
		return true, nil
	}
	if thr.MinJain > 0 && s.Jain < thr.MinJain {
		reasons = append(reasons, fmt.Sprintf("Jain %.4f < min %.4f", s.Jain, thr.MinJain))
	}
	if thr.MaxCV > 0 && s.CV > thr.MaxCV {
		reasons = append(reasons, fmt.Sprintf("CV %.4f > max %.4f", s.CV, thr.MaxCV))
	}
	if thr.MaxMaxMin > 0 && s.MaxMinRatio > thr.MaxMaxMin {
		reasons = append(reasons, fmt.Sprintf("Max/Min %.3f > max %.3f", s.MaxMinRatio, thr.MaxMaxMin))
	}
	if thr.MaxAbsSkew > 0 && math.Abs(s.Skew) > thr.MaxAbsSkew {
		reasons = append(reasons, fmt.Sprintf("|skew| %.3f > max %.3f", math.Abs(s.Skew), thr.MaxAbsSkew))
	}
	return len(reasons) == 0, reasons
}

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// TestPlacementStats verifies the statistical primitives against hand-computed
// values and known edge cases. It is pure (no cluster) and is the canonical
// proof that the fairness math the whole suite relies on is correct.
func TestPlacementStats(t *testing.T) {
	const eps = 1e-9

	t.Run("perfectly even", func(t *testing.T) {
		s := ComputeStats([]float64{4, 4, 4, 4})
		assert.InDelta(t, 4, s.Mean, eps)
		assert.InDelta(t, 0, s.StdDev, eps)
		assert.InDelta(t, 0, s.CV, eps)
		assert.InDelta(t, 1, s.Jain, eps)
		assert.InDelta(t, 1, s.MaxMinRatio, eps)
		assert.InDelta(t, 0, s.Skew, eps)
		ok, reasons := s.Check(EqualSizedThresholds())
		assert.True(t, ok, "even distribution must pass strict thresholds: %v", reasons)
	})

	t.Run("single hot entity", func(t *testing.T) {
		// All load on one of four entities is the worst case: Jain == 1/n.
		s := ComputeStats([]float64{12, 0, 0, 0})
		assert.InDelta(t, 0.25, s.Jain, eps)
		assert.True(t, math.IsInf(s.MaxMinRatio, 1))
		ok, _ := s.Check(EqualSizedThresholds())
		assert.False(t, ok, "single hot entity must fail fairness")
		ok, _ = s.Check(GeneralThresholds())
		assert.False(t, ok, "single hot entity must fail even relaxed fairness")
	})

	t.Run("imbalance is judged relative to sample size", func(t *testing.T) {
		// Identical absolute spread (+/-1 around the mean) becomes acceptable as
		// the counts grow: CV is scale-free, so few placements are inherently
		// noisier. This is why the integration suite sizes vdevs large enough to
		// give each entity several placements before applying the strict bar.
		small := ComputeStats([]float64{5, 5, 4, 6}) // CV = sqrt(0.5)/5 = 0.1414
		assert.InDelta(t, 5, small.Mean, eps)
		assert.InDelta(t, 0.5, small.Variance, eps)
		assert.InDelta(t, math.Sqrt(0.5), small.StdDev, eps)
		assert.InDelta(t, 1.5, small.MaxMinRatio, eps)
		ok, _ := small.Check(GeneralThresholds())
		assert.True(t, ok, "small +/-1 spread should pass the relaxed bar")
		ok, reasons := small.Check(EqualSizedThresholds())
		assert.False(t, ok, "the same spread is too noisy for the strict bar at low counts")
		assert.Contains(t, strings.Join(reasons, " "), "CV")

		large := ComputeStats([]float64{10, 10, 9, 11}) // CV = sqrt(0.5)/10 = 0.0707
		ok, reasons = large.Check(EqualSizedThresholds())
		assert.Truef(t, ok, "the same +/-1 spread at higher counts passes strict: %v", reasons)
	})

	t.Run("degenerate inputs", func(t *testing.T) {
		empty := ComputeStats(nil)
		assert.Equal(t, 0, empty.Count)
		ok, _ := empty.Check(EqualSizedThresholds())
		assert.True(t, ok, "empty sample set is vacuously fair")

		zeros := ComputeStats([]float64{0, 0, 0})
		assert.InDelta(t, 1, zeros.Jain, eps)
		assert.InDelta(t, 1, zeros.MaxMinRatio, eps)
		ok, _ = zeros.Check(EqualSizedThresholds())
		assert.True(t, ok, "all-zero sample set is vacuously fair")
	})

	t.Run("Jain spans its theoretical range", func(t *testing.T) {
		// For n samples Jain is bounded by [1/n, 1].
		best := ComputeStats([]float64{3, 3, 3})
		worst := ComputeStats([]float64{9, 0, 0})
		assert.InDelta(t, 1.0, best.Jain, eps)
		assert.InDelta(t, 1.0/3.0, worst.Jain, eps)
	})
}
