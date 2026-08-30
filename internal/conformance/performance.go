package conformance

import "fmt"

type PerformanceMetrics struct {
	LongFrameRatio        float64 `json:"longFrameRatio"`
	InputToNextFrameP95MS float64 `json:"inputToNextFrameP95Ms"`
	ScrollFrameP95MS      float64 `json:"scrollFrameP95Ms"`
	AnimationFrameP95MS   float64 `json:"animationFrameP95Ms"`
}

type PerformanceGate struct {
	Runner   string             `json:"runner"`
	Baseline PerformanceMetrics `json:"baseline"`
	Budget   PerformanceMetrics `json:"budget"`
	Current  PerformanceMetrics `json:"current"`
}

type PerformanceReport struct {
	Runner      string       `json:"runner"`
	Differences []Difference `json:"differences,omitempty"`
}

func (report PerformanceReport) Passed() bool {
	return report.Runner != "" && len(report.Differences) == 0
}

// ComparePerformance rejects both absolute budget violations and regressions
// from the checked-in v0.16.0 result on the same fixed runner.
func ComparePerformance(gate PerformanceGate) PerformanceReport {
	report := PerformanceReport{Runner: gate.Runner}
	metrics := []struct {
		name             string
		baseline, budget float64
		current          float64
	}{
		{"long-frame-ratio", gate.Baseline.LongFrameRatio, gate.Budget.LongFrameRatio, gate.Current.LongFrameRatio},
		{"input-to-next-frame-p95-ms", gate.Baseline.InputToNextFrameP95MS, gate.Budget.InputToNextFrameP95MS, gate.Current.InputToNextFrameP95MS},
		{"scroll-frame-p95-ms", gate.Baseline.ScrollFrameP95MS, gate.Budget.ScrollFrameP95MS, gate.Current.ScrollFrameP95MS},
		{"animation-frame-p95-ms", gate.Baseline.AnimationFrameP95MS, gate.Budget.AnimationFrameP95MS, gate.Current.AnimationFrameP95MS},
	}
	for _, metric := range metrics {
		if metric.current < 0 || metric.current > metric.budget {
			report.Differences = append(report.Differences, Difference{Category: "performance/budget", Subject: metric.name, Expected: lessOrEqual(metric.budget), Actual: number(metric.current)})
		}
		if metric.current > metric.baseline {
			report.Differences = append(report.Differences, Difference{Category: "performance/regression", Subject: metric.name, Expected: lessOrEqual(metric.baseline), Actual: number(metric.current)})
		}
	}
	return report
}

func lessOrEqual(value float64) string { return fmt.Sprintf("<= %.3f", value) }
