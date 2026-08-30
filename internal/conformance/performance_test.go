package conformance

import "testing"

func TestComparePerformanceRequiresBudgetAndNoV016Regression(t *testing.T) {
	gate := PerformanceGate{
		Runner:   "ubuntu-24.04-amd64-2vcpu",
		Baseline: PerformanceMetrics{LongFrameRatio: .01, InputToNextFrameP95MS: 12, ScrollFrameP95MS: 8, AnimationFrameP95MS: 9},
		Budget:   PerformanceMetrics{LongFrameRatio: .02, InputToNextFrameP95MS: 16, ScrollFrameP95MS: 10, AnimationFrameP95MS: 10},
		Current:  PerformanceMetrics{LongFrameRatio: .008, InputToNextFrameP95MS: 11, ScrollFrameP95MS: 7, AnimationFrameP95MS: 8},
	}
	if report := ComparePerformance(gate); !report.Passed() {
		t.Fatalf("non-regressing metrics failed: %+v", report)
	}
	gate.Current.InputToNextFrameP95MS = 13
	if report := ComparePerformance(gate); report.Passed() || !performanceDifference(report, "performance/regression", "input-to-next-frame-p95-ms") {
		t.Fatalf("baseline regression passed: %+v", report)
	}
	gate.Current.ScrollFrameP95MS = 11
	if report := ComparePerformance(gate); report.Passed() || !performanceDifference(report, "performance/budget", "scroll-frame-p95-ms") {
		t.Fatalf("absolute budget violation passed: %+v", report)
	}
}

func TestComparePerformanceRequiresFixedRunnerIdentity(t *testing.T) {
	if report := ComparePerformance(PerformanceGate{}); report.Passed() {
		t.Fatal("empty runner passed")
	}
}

func performanceDifference(report PerformanceReport, category, subject string) bool {
	for _, difference := range report.Differences {
		if difference.Category == category && difference.Subject == subject {
			return true
		}
	}
	return false
}
