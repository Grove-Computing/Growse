package animation

import (
	"errors"
	"math"
	"time"
)

// Phase identifies where a sample lies relative to an effect's active time.
type Phase uint8

const (
	PhaseBefore Phase = iota
	PhaseActive
	PhaseAfter
)

// EasingFunction maps an input progress to an output progress.
type EasingFunction interface {
	Transform(progress float64, before bool) float64
}

// Timing describes the duration, delay, and easing of one effect.
type Timing struct {
	Duration time.Duration
	Delay    time.Duration
	Easing   EasingFunction
}

// TimingSample is the phase and eased progress at one timestamp.
type TimingSample struct {
	Phase    Phase
	Progress float64
}

// NewTiming validates and creates effect timing parameters.
func NewTiming(duration, delay time.Duration, easing EasingFunction) (Timing, error) {
	if duration < 0 {
		return Timing{}, errors.New("animation duration must not be negative")
	}
	if easing == nil {
		easing = Linear{}
	}
	return Timing{Duration: duration, Delay: delay, Easing: easing}, nil
}

// Sample evaluates timing at current using start as the style-change time.
func (timing Timing) Sample(start, current time.Time) TimingSample {
	elapsed := current.Sub(start) - timing.Delay
	if elapsed < 0 {
		return TimingSample{Phase: PhaseBefore}
	}
	if timing.Duration == 0 || elapsed >= timing.Duration {
		return TimingSample{Phase: PhaseAfter, Progress: 1}
	}
	progress := float64(elapsed) / float64(timing.Duration)
	return TimingSample{Phase: PhaseActive, Progress: timing.Easing.Transform(progress, false)}
}

// Linear is the identity easing function.
type Linear struct{}

func (Linear) Transform(progress float64, _ bool) float64 {
	return progress
}

// CubicBezier is a CSS cubic-bezier easing function.
type CubicBezier struct {
	X1, Y1, X2, Y2 float64
}

// NewCubicBezier creates a curve whose x control points are valid CSS values.
func NewCubicBezier(x1, y1, x2, y2 float64) (CubicBezier, error) {
	values := []float64{x1, y1, x2, y2}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return CubicBezier{}, errors.New("cubic-bezier control points must be finite")
		}
	}
	if x1 < 0 || x1 > 1 || x2 < 0 || x2 > 1 {
		return CubicBezier{}, errors.New("cubic-bezier x control points must be between zero and one")
	}
	return CubicBezier{X1: x1, Y1: y1, X2: x2, Y2: y2}, nil
}

func (curve CubicBezier) Transform(progress float64, _ bool) float64 {
	if progress <= 0 {
		return 0
	}
	if progress >= 1 {
		return 1
	}

	parameter := progress
	for range 8 {
		x := bezierCoordinate(parameter, curve.X1, curve.X2) - progress
		if math.Abs(x) < 1e-7 {
			return bezierCoordinate(parameter, curve.Y1, curve.Y2)
		}
		derivative := bezierDerivative(parameter, curve.X1, curve.X2)
		if math.Abs(derivative) < 1e-7 {
			break
		}
		parameter -= x / derivative
		if parameter < 0 || parameter > 1 {
			break
		}
	}

	lower, upper := 0.0, 1.0
	for range 24 {
		parameter = (lower + upper) / 2
		x := bezierCoordinate(parameter, curve.X1, curve.X2)
		if math.Abs(x-progress) < 1e-7 {
			break
		}
		if x < progress {
			lower = parameter
		} else {
			upper = parameter
		}
	}
	return bezierCoordinate(parameter, curve.Y1, curve.Y2)
}

func bezierCoordinate(parameter, first, second float64) float64 {
	inverse := 1 - parameter
	return 3*inverse*inverse*parameter*first + 3*inverse*parameter*parameter*second + parameter*parameter*parameter
}

func bezierDerivative(parameter, first, second float64) float64 {
	inverse := 1 - parameter
	return 3*inverse*inverse*first + 6*inverse*parameter*(second-first) + 3*parameter*parameter*(1-second)
}

// StepPosition selects where discontinuities occur in a steps() easing.
type StepPosition uint8

const (
	JumpStart StepPosition = iota
	JumpEnd
	JumpNone
	JumpBoth
)

// Steps is a CSS step easing function.
type Steps struct {
	Count    int
	Position StepPosition
}

// NewSteps validates and creates a step easing function.
func NewSteps(count int, position StepPosition) (Steps, error) {
	if count <= 0 || (position == JumpNone && count <= 1) {
		return Steps{}, errors.New("steps count is invalid for the selected position")
	}
	if position > JumpBoth {
		return Steps{}, errors.New("unknown steps position")
	}
	return Steps{Count: count, Position: position}, nil
}

func (steps Steps) Transform(progress float64, before bool) float64 {
	current := math.Floor(progress * float64(steps.Count))
	if steps.Position == JumpStart || steps.Position == JumpBoth {
		current++
	}
	if before && progress*float64(steps.Count) == math.Trunc(progress*float64(steps.Count)) {
		current--
	}
	if progress >= 0 && current < 0 {
		current = 0
	}

	jumps := steps.Count
	if steps.Position == JumpNone {
		jumps--
	} else if steps.Position == JumpBoth {
		jumps++
	}
	if progress <= 1 && current > float64(jumps) {
		current = float64(jumps)
	}
	return current / float64(jumps)
}
