package style

import (
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/dom"
)

// TransitionValueKind identifies the representation used for interpolation.
type TransitionValueKind uint8

const (
	TransitionNumber TransitionValueKind = iota
	TransitionColor
	TransitionTransform
)

// TransitionValue is one supported computed value captured at a style change.
type TransitionValue struct {
	Kind      TransitionValueKind
	Number    float32
	Color     uint32
	Transform []TransformFunction
}

// StartedTransition describes a transition created by a computed-style change.
type StartedTransition struct {
	NodeID   dom.NodeID
	Property string
	From     TransitionValue
	To       TransitionValue
	Timing   animation.Timing
}

// RunningTransition retains the state needed to sample and interrupt one CSS
// transition without writing animated values back to author style.
type RunningTransition struct {
	StartedTransition
	StartTime                 time.Time
	Current                   TransitionValue
	ReversingAdjustedStart    TransitionValue
	ReversingShorteningFactor float64
}

// NewRunningTransition starts a transition at frameTime.
func NewRunningTransition(started StartedTransition, frameTime time.Time) *RunningTransition {
	return &RunningTransition{
		StartedTransition:         started,
		StartTime:                 frameTime,
		Current:                   cloneTransitionValue(started.From),
		ReversingAdjustedStart:    cloneTransitionValue(started.From),
		ReversingShorteningFactor: 1,
	}
}

// Advance samples the current value and reports whether the transition remains active.
func (running *RunningTransition) Advance(frameTime time.Time) (TransitionValue, bool) {
	sample := running.Timing.Sample(running.StartTime, frameTime)
	running.Current = interpolateTransitionValue(running.From, running.To, sample.Progress)
	return cloneTransitionValue(running.Current), sample.Phase != animation.PhaseAfter
}

// Interrupt starts a replacement transition from the currently displayed
// value. Returning to the reversing-adjusted start shortens the new duration.
func (running *RunningTransition) Interrupt(frameTime time.Time, target TransitionValue, timing animation.Timing) {
	current, _ := running.Advance(frameTime)
	factor := 1.0
	reversingStart := cloneTransitionValue(current)
	if reflect.DeepEqual(target, running.ReversingAdjustedStart) {
		oldSample := running.Timing.Sample(running.StartTime, frameTime)
		factor = math.Abs(oldSample.Progress*running.ReversingShorteningFactor + 1 - running.ReversingShorteningFactor)
		factor = min(max(factor, 0), 1)
		reversingStart = cloneTransitionValue(running.To)
	}
	timing.Duration = time.Duration(float64(timing.Duration) * factor)
	if timing.Delay < 0 {
		timing.Delay = time.Duration(float64(timing.Delay) * factor)
	}
	running.From = current
	running.To = cloneTransitionValue(target)
	running.Current = cloneTransitionValue(current)
	running.Timing = timing
	running.StartTime = frameTime
	running.ReversingAdjustedStart = reversingStart
	running.ReversingShorteningFactor = factor
}

func interpolateTransitionValue(from, to TransitionValue, progress float64) TransitionValue {
	if from.Kind != to.Kind {
		if progress >= 1 {
			return cloneTransitionValue(to)
		}
		return cloneTransitionValue(from)
	}
	switch from.Kind {
	case TransitionNumber:
		return TransitionValue{Kind: TransitionNumber, Number: from.Number + (to.Number-from.Number)*float32(progress)}
	default:
		if progress >= 1 {
			return cloneTransitionValue(to)
		}
		return cloneTransitionValue(from)
	}
}

func cloneTransitionValue(value TransitionValue) TransitionValue {
	value.Transform = append([]TransformFunction(nil), value.Transform...)
	return value
}

type transitionLists struct {
	properties []string
	durations  []time.Duration
	easings    []animation.EasingFunction
	delays     []time.Duration
}

var transitionableProperties = []string{
	"opacity", "transform", "color", "background-color",
	"border-top-color", "border-right-color", "border-bottom-color", "border-left-color", "outline-color",
}

// StartTransitions compares two computed-style snapshots and creates the
// transitions selected by the new snapshot. It does not mutate either map.
func StartTransitions(previous, next Map) []StartedTransition {
	var started []StartedTransition
	for nodeID, nextStyle := range next {
		previousStyle, exists := previous[nodeID]
		if !exists {
			continue
		}
		for _, property := range transitionableProperties {
			timing, enabled := transitionTimingForProperty(nextStyle.Transitions, property)
			if !enabled || timing.Duration <= 0 {
				continue
			}
			from, fromOK := computedTransitionValue(previousStyle, property)
			to, toOK := computedTransitionValue(nextStyle, property)
			if !fromOK || !toOK || reflect.DeepEqual(from, to) {
				continue
			}
			started = append(started, StartedTransition{
				NodeID: nodeID, Property: property, From: from, To: to, Timing: timing,
			})
		}
	}
	return started
}

func transitionTimingForProperty(transitions []Transition, property string) (animation.Timing, bool) {
	var selected animation.Timing
	found := false
	for _, transition := range transitions {
		if transition.Property == "all" || transition.Property == property {
			selected, found = transition.Timing, true
		}
	}
	return selected, found
}

func computedTransitionValue(computed ComputedStyle, property string) (TransitionValue, bool) {
	switch property {
	case "opacity":
		return TransitionValue{Kind: TransitionNumber, Number: computed.Opacity}, true
	case "transform":
		return TransitionValue{Kind: TransitionTransform, Transform: append([]TransformFunction(nil), computed.Transform...)}, true
	case "color":
		return TransitionValue{Kind: TransitionColor, Color: computed.Color}, true
	case "background-color":
		return TransitionValue{Kind: TransitionColor, Color: computed.BackgroundColor}, true
	case "border-top-color":
		return TransitionValue{Kind: TransitionColor, Color: computed.Border.Top.Color}, true
	case "border-right-color":
		return TransitionValue{Kind: TransitionColor, Color: computed.Border.Right.Color}, true
	case "border-bottom-color":
		return TransitionValue{Kind: TransitionColor, Color: computed.Border.Bottom.Color}, true
	case "border-left-color":
		return TransitionValue{Kind: TransitionColor, Color: computed.Border.Left.Color}, true
	case "outline-color":
		return TransitionValue{Kind: TransitionColor, Color: computed.Outline.Color}, true
	default:
		return TransitionValue{}, false
	}
}

func defaultTransitions() []Transition {
	timing, _ := animation.NewTiming(0, 0, defaultTransitionEasing())
	return []Transition{{Property: "all", Timing: timing}}
}

func defaultTransitionEasing() animation.EasingFunction {
	easing, _ := animation.NewCubicBezier(0.25, 0.1, 0.25, 1)
	return easing
}

func applyTransitionProperties(computed, parent ComputedStyle, winners map[string]winner, custom map[string]string) ComputedStyle {
	lists := transitionLists{
		properties: []string{"all"}, durations: []time.Duration{0},
		easings: []animation.EasingFunction{defaultTransitionEasing()}, delays: []time.Duration{0},
	}
	parentLists := transitionListsFromComputed(parent.Transitions)
	for _, property := range []string{"transition-property", "transition-duration", "transition-timing-function", "transition-delay"} {
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

		var parsed transitionLists
		if candidate.source == "transition" {
			parsed, valid = parseTransitionShorthand(value)
		} else {
			parsed, valid = parseTransitionLonghand(property, value)
		}
		if valid {
			lists.set(property, parsed)
		}
	}

	if len(lists.properties) == 0 {
		computed.Transitions = nil
		return computed
	}
	computed.Transitions = make([]Transition, len(lists.properties))
	for index, property := range lists.properties {
		timing, _ := animation.NewTiming(
			lists.durations[index%len(lists.durations)],
			lists.delays[index%len(lists.delays)],
			lists.easings[index%len(lists.easings)],
		)
		computed.Transitions[index] = Transition{Property: property, Timing: timing}
	}
	return computed
}

func (lists *transitionLists) set(property string, source transitionLists) {
	switch property {
	case "transition-property":
		lists.properties = append([]string(nil), source.properties...)
	case "transition-duration":
		lists.durations = append([]time.Duration(nil), source.durations...)
	case "transition-timing-function":
		lists.easings = append([]animation.EasingFunction(nil), source.easings...)
	case "transition-delay":
		lists.delays = append([]time.Duration(nil), source.delays...)
	}
}

func transitionListsFromComputed(transitions []Transition) transitionLists {
	if len(transitions) == 0 {
		return transitionLists{easings: []animation.EasingFunction{defaultTransitionEasing()}, durations: []time.Duration{0}, delays: []time.Duration{0}}
	}
	lists := transitionLists{}
	for _, transition := range transitions {
		lists.properties = append(lists.properties, transition.Property)
		lists.durations = append(lists.durations, transition.Timing.Duration)
		lists.easings = append(lists.easings, transition.Timing.Easing)
		lists.delays = append(lists.delays, transition.Timing.Delay)
	}
	return lists
}

func parseTransitionLonghand(property, value string) (transitionLists, bool) {
	parts := splitBackgroundArguments(value)
	if len(parts) == 0 {
		return transitionLists{}, false
	}
	result := transitionLists{}
	for _, part := range parts {
		switch property {
		case "transition-property":
			name := strings.ToLower(strings.TrimSpace(part))
			if name == "none" {
				if len(parts) != 1 {
					return transitionLists{}, false
				}
				return transitionLists{}, true
			}
			if !validTransitionProperty(name) {
				return transitionLists{}, false
			}
			result.properties = append(result.properties, name)
		case "transition-duration", "transition-delay":
			duration, recognized, valid := parseTransitionTime(part, property == "transition-delay")
			if !recognized || !valid {
				return transitionLists{}, false
			}
			if property == "transition-duration" {
				result.durations = append(result.durations, duration)
			} else {
				result.delays = append(result.delays, duration)
			}
		case "transition-timing-function":
			easing, valid := parseEasingFunction(part)
			if !valid {
				return transitionLists{}, false
			}
			result.easings = append(result.easings, easing)
		}
	}
	return result, true
}

func parseTransitionShorthand(value string) (transitionLists, bool) {
	items := splitBackgroundArguments(value)
	if len(items) == 0 {
		return transitionLists{}, false
	}
	result := transitionLists{}
	for _, item := range items {
		parts, valid := splitCSSSpaceSeparated(item)
		if !valid || len(parts) == 0 {
			return transitionLists{}, false
		}
		property := "all"
		duration, delay := time.Duration(0), time.Duration(0)
		easing := defaultTransitionEasing()
		propertySet, easingSet, times := false, false, 0
		for _, part := range parts {
			if parsed, recognized, timeValid := parseTransitionTime(part, times > 0); recognized {
				if !timeValid || times >= 2 {
					return transitionLists{}, false
				}
				if times == 0 {
					duration = parsed
				} else {
					delay = parsed
				}
				times++
				continue
			}
			if parsed, ok := parseEasingFunction(part); ok {
				if easingSet {
					return transitionLists{}, false
				}
				easing, easingSet = parsed, true
				continue
			}
			name := strings.ToLower(strings.TrimSpace(part))
			if propertySet || !validTransitionProperty(name) {
				return transitionLists{}, false
			}
			property, propertySet = name, true
		}
		if property == "none" {
			if len(items) != 1 {
				return transitionLists{}, false
			}
			return transitionLists{}, true
		}
		result.properties = append(result.properties, property)
		result.durations = append(result.durations, duration)
		result.easings = append(result.easings, easing)
		result.delays = append(result.delays, delay)
	}
	return result, true
}

func parseTransitionTime(value string, allowNegative bool) (time.Duration, bool, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	multiplier := time.Duration(0)
	number := ""
	if strings.HasSuffix(value, "ms") {
		multiplier, number = time.Millisecond, strings.TrimSuffix(value, "ms")
	} else if strings.HasSuffix(value, "s") {
		multiplier, number = time.Second, strings.TrimSuffix(value, "s")
	} else {
		return 0, false, false
	}
	parsed, err := strconv.ParseFloat(number, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false, false
	}
	if !allowNegative && parsed < 0 {
		return 0, true, false
	}
	return time.Duration(parsed * float64(multiplier)), true, true
}

func parseEasingFunction(value string) (animation.EasingFunction, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "linear":
		return animation.Linear{}, true
	case "ease":
		return defaultTransitionEasing(), true
	case "ease-in":
		result, _ := animation.NewCubicBezier(0.42, 0, 1, 1)
		return result, true
	case "ease-out":
		result, _ := animation.NewCubicBezier(0, 0, 0.58, 1)
		return result, true
	case "ease-in-out":
		result, _ := animation.NewCubicBezier(0.42, 0, 0.58, 1)
		return result, true
	case "step-start":
		result, _ := animation.NewSteps(1, animation.JumpStart)
		return result, true
	case "step-end":
		result, _ := animation.NewSteps(1, animation.JumpEnd)
		return result, true
	}
	if strings.HasPrefix(value, "cubic-bezier(") && strings.HasSuffix(value, ")") {
		parts := splitBackgroundArguments(value[len("cubic-bezier(") : len(value)-1])
		if len(parts) != 4 {
			return nil, false
		}
		points := [4]float64{}
		for index, part := range parts {
			var err error
			points[index], err = strconv.ParseFloat(strings.TrimSpace(part), 64)
			if err != nil {
				return nil, false
			}
		}
		result, err := animation.NewCubicBezier(points[0], points[1], points[2], points[3])
		return result, err == nil
	}
	if strings.HasPrefix(value, "steps(") && strings.HasSuffix(value, ")") {
		parts := splitBackgroundArguments(value[len("steps(") : len(value)-1])
		if len(parts) < 1 || len(parts) > 2 {
			return nil, false
		}
		count, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, false
		}
		position := animation.JumpEnd
		if len(parts) == 2 {
			switch strings.ToLower(strings.TrimSpace(parts[1])) {
			case "jump-start", "start":
				position = animation.JumpStart
			case "jump-end", "end":
				position = animation.JumpEnd
			case "jump-none":
				position = animation.JumpNone
			case "jump-both":
				position = animation.JumpBoth
			default:
				return nil, false
			}
		}
		result, err := animation.NewSteps(count, position)
		return result, err == nil
	}
	return nil, false
}

func validTransitionProperty(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || character == '-' || character == '_' || (index > 0 && character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}
