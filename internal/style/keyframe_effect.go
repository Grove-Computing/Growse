package style

import (
	"sort"
	"strings"
	"time"

	animationmodel "github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/css"
)

// AnimatedStyles samples all registered keyframe animations and composites
// them over the immutable underlying computed styles.
func (registry *AnimationRegistry) AnimatedStyles(underlying Map, stylesheet *css.Stylesheet, current time.Time) Map {
	result := make(Map, len(underlying))
	for nodeID, computed := range underlying {
		result[nodeID] = computed
	}
	if registry == nil || stylesheet == nil {
		return result
	}
	for nodeID, stack := range registry.stacks {
		base, exists := underlying[nodeID]
		if !exists {
			continue
		}
		values := AnimatedValues{}
		for _, sampled := range stack.Sample(current) {
			if !sampled.Sample.Applies {
				continue
			}
			rule, ok := keyframesByName(stylesheet, sampled.Name)
			if !ok {
				continue
			}
			for property, value := range sampleKeyframeRule(rule, sampled.Sample.Progress, base) {
				values[property] = value
			}
		}
		result[nodeID] = ApplyAnimatedCascade(base, values, nil)
	}
	return result
}

// Active reports whether an animation can change on a future frame.
func (registry *AnimationRegistry) Active(current time.Time) bool {
	if registry == nil {
		return false
	}
	for _, stack := range registry.stacks {
		for _, running := range stack.items {
			if running.Paused() {
				continue
			}
			if running.Sample(current).Phase != animationmodel.PhaseAfter {
				return true
			}
		}
	}
	return false
}

func keyframesByName(stylesheet *css.Stylesheet, name string) (css.KeyframesRule, bool) {
	for index := len(stylesheet.Keyframes) - 1; index >= 0; index-- {
		if stylesheet.Keyframes[index].Name == name {
			return stylesheet.Keyframes[index], true
		}
	}
	return css.KeyframesRule{}, false
}

type keyframeStop struct {
	offset float64
	value  TransitionValue
}

func sampleKeyframeRule(rule css.KeyframesRule, progress float64, underlying ComputedStyle) AnimatedValues {
	properties := make(map[string]map[float64]TransitionValue)
	for _, frame := range rule.Frames {
		for _, declaration := range frame.Declarations {
			if declaration.Important {
				continue
			}
			property := strings.ToLower(declaration.Property)
			value, ok := keyframeTransitionValue(property, declaration.Value.Raw, underlying)
			if !ok {
				continue
			}
			if properties[property] == nil {
				properties[property] = make(map[float64]TransitionValue)
			}
			for _, offset := range frame.Offsets {
				properties[property][offset] = value
			}
		}
	}
	result := AnimatedValues{}
	for property, byOffset := range properties {
		base, hasBase := computedTransitionValue(underlying, property)
		if _, exists := byOffset[0]; !exists {
			if !hasBase {
				continue
			}
			byOffset[0] = base
		}
		if _, exists := byOffset[1]; !exists {
			if !hasBase {
				continue
			}
			byOffset[1] = base
		}
		stops := make([]keyframeStop, 0, len(byOffset))
		for offset, value := range byOffset {
			stops = append(stops, keyframeStop{offset: offset, value: value})
		}
		sort.Slice(stops, func(left, right int) bool { return stops[left].offset < stops[right].offset })
		left, right := stops[0], stops[len(stops)-1]
		for index := range stops {
			if stops[index].offset <= progress {
				left = stops[index]
			}
			if stops[index].offset >= progress {
				right = stops[index]
				break
			}
		}
		localProgress := 0.0
		if right.offset > left.offset {
			localProgress = (progress - left.offset) / (right.offset - left.offset)
		}
		result[property] = interpolateTransitionValue(left.value, right.value, localProgress)
	}
	return result
}

func keyframeTransitionValue(property, raw string, underlying ComputedStyle) (TransitionValue, bool) {
	switch property {
	case "opacity":
		value, ok := parseOpacity(strings.TrimSpace(raw))
		return TransitionValue{Kind: TransitionNumber, Number: value}, ok
	case "transform":
		value, ok := parseTransform(raw, LengthContext{FontSize: underlying.FontSize, RootFontSize: 16})
		return TransitionValue{Kind: TransitionTransform, Transform: value}, ok
	case "color", "background-color", "border-top-color", "border-right-color", "border-bottom-color", "border-left-color", "outline-color":
		value, ok := parseColor(raw, underlying.Color)
		return TransitionValue{Kind: TransitionColor, Color: value}, ok
	case "width", "height":
		value, ok := ResolveLength(strings.TrimSpace(raw), LengthContext{FontSize: underlying.FontSize, RootFontSize: 16})
		return TransitionValue{Kind: TransitionLength, Length: value}, ok
	default:
		return TransitionValue{}, false
	}
}
