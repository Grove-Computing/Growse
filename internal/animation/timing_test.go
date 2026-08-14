package animation

import (
	"math"
	"testing"
	"time"
)

func TestTimingEvaluatesDurationAndPositiveDelay(t *testing.T) {
	start := time.Unix(100, 0)
	timing, err := NewTiming(time.Second, 250*time.Millisecond, Linear{})
	if err != nil {
		t.Fatal(err)
	}

	assertTimingSample(t, timing.Sample(start, start.Add(200*time.Millisecond)), PhaseBefore, 0)
	assertTimingSample(t, timing.Sample(start, start.Add(750*time.Millisecond)), PhaseActive, 0.5)
	assertTimingSample(t, timing.Sample(start, start.Add(1250*time.Millisecond)), PhaseAfter, 1)
}

func TestTimingNegativeDelayStartsPartwayThrough(t *testing.T) {
	start := time.Unix(100, 0)
	timing, err := NewTiming(time.Second, -250*time.Millisecond, Linear{})
	if err != nil {
		t.Fatal(err)
	}

	assertTimingSample(t, timing.Sample(start, start), PhaseActive, 0.25)
	assertTimingSample(t, timing.Sample(start, start.Add(750*time.Millisecond)), PhaseAfter, 1)
}

func TestTimingRejectsNegativeDuration(t *testing.T) {
	if _, err := NewTiming(-time.Millisecond, 0, Linear{}); err == nil {
		t.Fatal("negative duration was accepted")
	}
}

func TestCubicBezierEasing(t *testing.T) {
	easeIn, err := NewCubicBezier(0.42, 0, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := easeIn.Transform(0.5, false), 0.3153568; math.Abs(got-want) > 1e-5 {
		t.Fatalf("ease-in at 0.5 = %.7f, want %.7f", got, want)
	}
	if _, err := NewCubicBezier(-0.1, 0, 1, 1); err == nil {
		t.Fatal("out-of-range cubic-bezier x coordinate was accepted")
	}
	if _, err := NewCubicBezier(0, math.Inf(1), 1, 1); err == nil {
		t.Fatal("non-finite cubic-bezier coordinate was accepted")
	}
}

func TestStepEasingPositionsAndBeforeFlag(t *testing.T) {
	jumpEnd, err := NewSteps(4, JumpEnd)
	if err != nil {
		t.Fatal(err)
	}
	if got := jumpEnd.Transform(0.5, false); got != 0.5 {
		t.Fatalf("steps(4, jump-end) at 0.5 = %v, want 0.5", got)
	}
	if got := jumpEnd.Transform(0.5, true); got != 0.25 {
		t.Fatalf("steps(4, jump-end) before boundary = %v, want 0.25", got)
	}

	jumpBoth, err := NewSteps(4, JumpBoth)
	if err != nil {
		t.Fatal(err)
	}
	if got := jumpBoth.Transform(0, false); got != 0.2 {
		t.Fatalf("steps(4, jump-both) at 0 = %v, want 0.2", got)
	}
	if _, err := NewSteps(1, JumpNone); err == nil {
		t.Fatal("steps(1, jump-none) was accepted")
	}
}

func assertTimingSample(t *testing.T, sample TimingSample, phase Phase, progress float64) {
	t.Helper()
	if sample.Phase != phase || math.Abs(sample.Progress-progress) > 1e-9 {
		t.Fatalf("sample = {phase:%v progress:%v}, want {phase:%v progress:%v}", sample.Phase, sample.Progress, phase, progress)
	}
}
