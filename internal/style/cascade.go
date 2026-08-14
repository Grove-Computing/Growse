package style

// AnimatedValues stores sampled values by CSS property name.
type AnimatedValues map[string]TransitionValue

// ApplyAnimatedCascade composites sampled CSS Animation and Transition values
// over an underlying computed style. CSS Animations override normal author
// declarations, author !important overrides Animations, and Transitions
// override both. The underlying style is not modified.
func ApplyAnimatedCascade(underlying ComputedStyle, animations, transitions AnimatedValues) ComputedStyle {
	result := underlying
	for property, value := range animations {
		if underlying.Important(property) {
			continue
		}
		result = applyAnimatedValue(result, property, value)
	}
	for property, value := range transitions {
		result = applyAnimatedValue(result, property, value)
	}
	return result
}

// ApplyAnimationSample applies an animation effect only while its timing and
// fill mode say the effect participates in the cascade.
func ApplyAnimationSample(underlying ComputedStyle, values AnimatedValues, sample AnimationSample) ComputedStyle {
	if !sample.Applies {
		return underlying
	}
	return ApplyAnimatedCascade(underlying, values, nil)
}

func applyAnimatedValue(computed ComputedStyle, property string, value TransitionValue) ComputedStyle {
	switch property {
	case "opacity":
		if value.Kind == TransitionNumber {
			computed.Opacity = min(max(value.Number, 0), 1)
		}
	case "transform":
		if value.Kind == TransitionTransform {
			computed.Transform = append([]TransformFunction(nil), value.Transform...)
		}
	case "color":
		if value.Kind == TransitionColor {
			computed.Color = value.Color
		}
	case "background-color":
		if value.Kind == TransitionColor {
			computed.BackgroundColor = value.Color
		}
	case "border-top-color":
		if value.Kind == TransitionColor {
			computed.Border.Top.Color = value.Color
		}
	case "border-right-color":
		if value.Kind == TransitionColor {
			computed.Border.Right.Color = value.Color
		}
	case "border-bottom-color":
		if value.Kind == TransitionColor {
			computed.Border.Bottom.Color = value.Color
		}
	case "border-left-color":
		if value.Kind == TransitionColor {
			computed.Border.Left.Color = value.Color
		}
	case "outline-color":
		if value.Kind == TransitionColor {
			computed.Outline.Color = value.Color
		}
	}
	return computed
}
