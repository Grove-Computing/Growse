package style

import (
	"math"
	"strconv"
	"strings"
	"time"

	animationmodel "github.com/Grove-Computing/Growse/internal/animation"
)

type animationLists struct {
	names      []string
	durations  []time.Duration
	easings    []animationmodel.EasingFunction
	delays     []time.Duration
	iterations []float64
	directions []AnimationDirection
	fills      []AnimationFillMode
	plays      []AnimationPlayState
}

// AnimationSample is one CSS animation's directed and eased progress.
type AnimationSample struct {
	Phase     animationmodel.Phase
	Progress  float64
	Iteration uint64
	Applies   bool
}

// RunningAnimation tracks pause time separately from the CSS timing values.
type RunningAnimation struct {
	Animation   CSSAnimation
	StartTime   time.Time
	paused      bool
	pausedAt    time.Time
	totalPaused time.Duration
}

// NewRunningAnimation creates an animation whose timeline begins at start.
func NewRunningAnimation(animation CSSAnimation, start time.Time) *RunningAnimation {
	running := &RunningAnimation{Animation: animation, StartTime: start}
	if animation.PlayState == AnimationPaused {
		running.paused = true
		running.pausedAt = start
	}
	return running
}

// Sample returns progress without advancing time while paused.
func (running *RunningAnimation) Sample(current time.Time) AnimationSample {
	effective := current
	if running.paused {
		effective = running.pausedAt
	}
	effective = effective.Add(-running.totalPaused)
	return running.Animation.Sample(running.StartTime, effective)
}

// Pause holds the current progress. Repeated calls have no effect.
func (running *RunningAnimation) Pause(current time.Time) {
	if running.paused {
		return
	}
	running.paused = true
	running.pausedAt = current
	running.Animation.PlayState = AnimationPaused
}

// Resume continues from the held progress. Repeated calls have no effect.
func (running *RunningAnimation) Resume(current time.Time) {
	if !running.paused {
		return
	}
	if current.After(running.pausedAt) {
		running.totalPaused += current.Sub(running.pausedAt)
	}
	running.paused = false
	running.Animation.PlayState = AnimationRunning
}

// Paused reports whether timeline progress is currently held.
func (running *RunningAnimation) Paused() bool {
	return running.paused
}

// Sample evaluates delay, iteration count, direction, fill mode, and easing.
func (item CSSAnimation) Sample(start, current time.Time) AnimationSample {
	elapsed := current.Sub(start) - item.Timing.Delay
	if elapsed < 0 {
		progress := directedAnimationProgress(0, 0, item.Direction)
		return AnimationSample{
			Phase: animationmodel.PhaseBefore, Progress: item.Timing.Easing.Transform(progress, true),
			Applies: item.FillMode == AnimationFillBackwards || item.FillMode == AnimationFillBoth,
		}
	}
	if item.Timing.Duration <= 0 || item.Iterations <= 0 {
		return item.afterSample()
	}

	overall := float64(elapsed) / float64(item.Timing.Duration)
	if !math.IsInf(item.Iterations, 1) && overall >= item.Iterations {
		return item.afterSample()
	}
	iteration := uint64(math.Floor(overall))
	progress := overall - math.Floor(overall)
	progress = directedAnimationProgress(progress, iteration, item.Direction)
	return AnimationSample{
		Phase: animationmodel.PhaseActive, Progress: item.Timing.Easing.Transform(progress, false),
		Iteration: iteration, Applies: true,
	}
}

func (item CSSAnimation) afterSample() AnimationSample {
	iteration, progress := uint64(0), 0.0
	if item.Iterations > 0 && !math.IsInf(item.Iterations, 1) {
		whole, fraction := math.Modf(item.Iterations)
		if fraction == 0 {
			iteration = uint64(max(whole-1, 0))
			progress = 1
		} else {
			iteration = uint64(whole)
			progress = fraction
		}
	}
	progress = directedAnimationProgress(progress, iteration, item.Direction)
	return AnimationSample{
		Phase: animationmodel.PhaseAfter, Progress: item.Timing.Easing.Transform(progress, false),
		Iteration: iteration, Applies: item.FillMode == AnimationFillForwards || item.FillMode == AnimationFillBoth,
	}
}

func directedAnimationProgress(progress float64, iteration uint64, direction AnimationDirection) float64 {
	reversed := direction == AnimationReverse || direction == AnimationAlternateReverse
	if (direction == AnimationAlternate || direction == AnimationAlternateReverse) && iteration%2 == 1 {
		reversed = !reversed
	}
	if reversed {
		return 1 - progress
	}
	return progress
}

func defaultAnimations() []CSSAnimation {
	timing, _ := animationmodel.NewTiming(0, 0, defaultTransitionEasing())
	return []CSSAnimation{{Name: "none", Timing: timing, Iterations: 1}}
}

func defaultAnimationLists() animationLists {
	return animationLists{
		names: []string{"none"}, durations: []time.Duration{0}, easings: []animationmodel.EasingFunction{defaultTransitionEasing()},
		delays: []time.Duration{0}, iterations: []float64{1}, directions: []AnimationDirection{AnimationNormal},
		fills: []AnimationFillMode{AnimationFillNone}, plays: []AnimationPlayState{AnimationRunning},
	}
}

func applyAnimationProperties(computed, parent ComputedStyle, winners map[string]winner, custom map[string]string) ComputedStyle {
	lists := defaultAnimationLists()
	parentLists := animationListsFromComputed(parent.Animations)
	properties := []string{
		"animation-name", "animation-duration", "animation-timing-function", "animation-delay",
		"animation-iteration-count", "animation-direction", "animation-fill-mode", "animation-play-state",
	}
	for _, property := range properties {
		candidate, exists := winners[property]
		if !exists {
			continue
		}
		value, valid := winnerValue(candidate, custom)
		if !valid {
			continue
		}
		switch parseGlobalKeyword(value) {
		case globalInherit:
			lists.set(property, parentLists)
			continue
		case globalInitial, globalUnset:
			continue
		}
		var parsed animationLists
		if candidate.source == "animation" {
			parsed, valid = parseAnimationShorthand(value)
		} else {
			parsed, valid = parseAnimationLonghand(property, value)
		}
		if valid {
			lists.set(property, parsed)
		}
	}

	computed.Animations = make([]CSSAnimation, len(lists.names))
	for index, name := range lists.names {
		timing, _ := animationmodel.NewTiming(
			lists.durations[index%len(lists.durations)], lists.delays[index%len(lists.delays)], lists.easings[index%len(lists.easings)],
		)
		computed.Animations[index] = CSSAnimation{
			Name: name, Timing: timing, Iterations: lists.iterations[index%len(lists.iterations)],
			Direction: lists.directions[index%len(lists.directions)], FillMode: lists.fills[index%len(lists.fills)],
			PlayState: lists.plays[index%len(lists.plays)],
		}
	}
	return computed
}

func (lists *animationLists) set(property string, source animationLists) {
	switch property {
	case "animation-name":
		lists.names = append([]string(nil), source.names...)
	case "animation-duration":
		lists.durations = append([]time.Duration(nil), source.durations...)
	case "animation-timing-function":
		lists.easings = append([]animationmodel.EasingFunction(nil), source.easings...)
	case "animation-delay":
		lists.delays = append([]time.Duration(nil), source.delays...)
	case "animation-iteration-count":
		lists.iterations = append([]float64(nil), source.iterations...)
	case "animation-direction":
		lists.directions = append([]AnimationDirection(nil), source.directions...)
	case "animation-fill-mode":
		lists.fills = append([]AnimationFillMode(nil), source.fills...)
	case "animation-play-state":
		lists.plays = append([]AnimationPlayState(nil), source.plays...)
	}
}

func animationListsFromComputed(animations []CSSAnimation) animationLists {
	if len(animations) == 0 {
		return defaultAnimationLists()
	}
	result := animationLists{}
	for _, item := range animations {
		result.names = append(result.names, item.Name)
		result.durations = append(result.durations, item.Timing.Duration)
		result.easings = append(result.easings, item.Timing.Easing)
		result.delays = append(result.delays, item.Timing.Delay)
		result.iterations = append(result.iterations, item.Iterations)
		result.directions = append(result.directions, item.Direction)
		result.fills = append(result.fills, item.FillMode)
		result.plays = append(result.plays, item.PlayState)
	}
	return result
}

func parseAnimationLonghand(property, value string) (animationLists, bool) {
	parts := splitBackgroundArguments(value)
	if len(parts) == 0 {
		return animationLists{}, false
	}
	result := animationLists{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch property {
		case "animation-name":
			if !validAnimationName(part) {
				return animationLists{}, false
			}
			result.names = append(result.names, part)
		case "animation-duration", "animation-delay":
			parsed, recognized, valid := parseTransitionTime(part, property == "animation-delay")
			if !recognized || !valid {
				return animationLists{}, false
			}
			if property == "animation-duration" {
				result.durations = append(result.durations, parsed)
			} else {
				result.delays = append(result.delays, parsed)
			}
		case "animation-timing-function":
			parsed, valid := parseEasingFunction(part)
			if !valid {
				return animationLists{}, false
			}
			result.easings = append(result.easings, parsed)
		case "animation-iteration-count":
			parsed, valid := parseIterationCount(part)
			if !valid {
				return animationLists{}, false
			}
			result.iterations = append(result.iterations, parsed)
		case "animation-direction":
			parsed, valid := parseAnimationDirection(part)
			if !valid {
				return animationLists{}, false
			}
			result.directions = append(result.directions, parsed)
		case "animation-fill-mode":
			parsed, valid := parseAnimationFill(part)
			if !valid {
				return animationLists{}, false
			}
			result.fills = append(result.fills, parsed)
		case "animation-play-state":
			parsed, valid := parseAnimationPlay(part)
			if !valid {
				return animationLists{}, false
			}
			result.plays = append(result.plays, parsed)
		}
	}
	return result, true
}

func parseAnimationShorthand(value string) (animationLists, bool) {
	items := splitBackgroundArguments(value)
	if len(items) == 0 {
		return animationLists{}, false
	}
	result := animationLists{}
	for _, item := range items {
		parts, valid := splitCSSSpaceSeparated(item)
		if !valid || len(parts) == 0 {
			return animationLists{}, false
		}
		name := "none"
		duration, delay := time.Duration(0), time.Duration(0)
		easing := defaultTransitionEasing()
		iterations := 1.0
		direction, fill, play := AnimationNormal, AnimationFillNone, AnimationRunning
		nameSet, easingSet, iterationSet, directionSet, fillSet, playSet, times := false, false, false, false, false, false, 0
		for _, part := range parts {
			if parsed, recognized, timeValid := parseTransitionTime(part, times > 0); recognized {
				if !timeValid || times >= 2 {
					return animationLists{}, false
				}
				if times == 0 {
					duration = parsed
				} else {
					delay = parsed
				}
				times++
				continue
			}
			if parsed, ok := parseEasingFunction(part); ok && !easingSet {
				easing, easingSet = parsed, true
				continue
			}
			if parsed, ok := parseIterationCount(part); ok && !iterationSet {
				iterations, iterationSet = parsed, true
				continue
			}
			if parsed, ok := parseAnimationDirection(part); ok && !directionSet {
				direction, directionSet = parsed, true
				continue
			}
			if parsed, ok := parseAnimationFill(part); ok && !fillSet {
				fill, fillSet = parsed, true
				continue
			}
			if parsed, ok := parseAnimationPlay(part); ok && !playSet {
				play, playSet = parsed, true
				continue
			}
			if nameSet || !validAnimationName(part) {
				return animationLists{}, false
			}
			name, nameSet = part, true
		}
		result.names = append(result.names, name)
		result.durations = append(result.durations, duration)
		result.easings = append(result.easings, easing)
		result.delays = append(result.delays, delay)
		result.iterations = append(result.iterations, iterations)
		result.directions = append(result.directions, direction)
		result.fills = append(result.fills, fill)
		result.plays = append(result.plays, play)
	}
	return result, true
}

func validAnimationName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "{};,()") {
		return false
	}
	return validTransitionProperty(strings.ToLower(value))
}

func parseIterationCount(value string) (float64, bool) {
	if strings.EqualFold(strings.TrimSpace(value), "infinite") {
		return math.Inf(1), true
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) && parsed >= 0
}

func parseAnimationDirection(value string) (AnimationDirection, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "normal":
		return AnimationNormal, true
	case "reverse":
		return AnimationReverse, true
	case "alternate":
		return AnimationAlternate, true
	case "alternate-reverse":
		return AnimationAlternateReverse, true
	default:
		return 0, false
	}
}

func parseAnimationFill(value string) (AnimationFillMode, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none":
		return AnimationFillNone, true
	case "forwards":
		return AnimationFillForwards, true
	case "backwards":
		return AnimationFillBackwards, true
	case "both":
		return AnimationFillBoth, true
	default:
		return 0, false
	}
}

func parseAnimationPlay(value string) (AnimationPlayState, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "running":
		return AnimationRunning, true
	case "paused":
		return AnimationPaused, true
	default:
		return 0, false
	}
}
