package style

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestClassifyAnimationDamageSeparatesCompositePaintAndLayout(t *testing.T) {
	nodeID := dom.NodeID(7)
	base := ComputedStyle{Opacity: 1, Color: 0xff0000ff, FontSize: 16}
	underlying := Map{nodeID: base}

	composite := base
	composite.Opacity = 0.5
	composite.Transform = []TransformFunction{{Kind: TransformTranslate, X: LengthPercentage{Pixels: 20}}}
	if got := ClassifyAnimationDamage(underlying, Map{nodeID: composite}); got != AnimationDamageComposite {
		t.Fatalf("composite damage = %v, want %v", got, AnimationDamageComposite)
	}

	painted := composite
	painted.Color = 0x0000ffff
	if got := ClassifyAnimationDamage(underlying, Map{nodeID: painted}); got != AnimationDamagePaint {
		t.Fatalf("paint damage = %v, want %v", got, AnimationDamagePaint)
	}

	layout := base
	layout.FontSize = 24
	if got := ClassifyAnimationDamage(underlying, Map{nodeID: layout}); got != AnimationDamageLayout {
		t.Fatalf("layout damage = %v, want %v", got, AnimationDamageLayout)
	}
}
