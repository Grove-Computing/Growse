package style

import "reflect"

// AnimationDamage classifies the most expensive rendering stage that a
// sampled animation frame must update.
type AnimationDamage uint8

const (
	AnimationDamageNone AnimationDamage = iota
	AnimationDamageComposite
	AnimationDamagePaint
	AnimationDamageLayout
)

// ClassifyAnimationDamage compares immutable computed styles with one sampled
// frame. Transform and opacity can be composited over cached geometry, colors
// need paint-command updates, and every other computed change requires layout.
func ClassifyAnimationDamage(underlying, sampled Map) AnimationDamage {
	damage := AnimationDamageNone
	for nodeID, current := range sampled {
		base, exists := underlying[nodeID]
		if !exists {
			return AnimationDamageLayout
		}
		if base.Opacity != current.Opacity || !reflect.DeepEqual(base.Transform, current.Transform) {
			damage = maxAnimationDamage(damage, AnimationDamageComposite)
		}
		if animationPaintChanged(base, current) {
			damage = maxAnimationDamage(damage, AnimationDamagePaint)
		}
		base = withoutAnimationPaintState(base)
		current = withoutAnimationPaintState(current)
		if !reflect.DeepEqual(base, current) {
			return AnimationDamageLayout
		}
	}
	if len(sampled) != len(underlying) {
		return AnimationDamageLayout
	}
	return damage
}

func animationPaintChanged(base, current ComputedStyle) bool {
	return base.Color != current.Color ||
		base.BackgroundColor != current.BackgroundColor ||
		base.Border.Top.Color != current.Border.Top.Color ||
		base.Border.Right.Color != current.Border.Right.Color ||
		base.Border.Bottom.Color != current.Border.Bottom.Color ||
		base.Border.Left.Color != current.Border.Left.Color ||
		base.Outline.Color != current.Outline.Color
}

func withoutAnimationPaintState(computed ComputedStyle) ComputedStyle {
	computed.Opacity = 0
	computed.Transform = nil
	computed.Color = 0
	computed.BackgroundColor = 0
	computed.Border.Top.Color = 0
	computed.Border.Right.Color = 0
	computed.Border.Bottom.Color = 0
	computed.Border.Left.Color = 0
	computed.Outline.Color = 0
	return computed
}

func maxAnimationDamage(left, right AnimationDamage) AnimationDamage {
	if right > left {
		return right
	}
	return left
}
