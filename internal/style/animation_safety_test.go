package style

import (
	"math"
	"testing"
	"time"

	animationmodel "github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestAnimationSafetyLimits(t *testing.T) {
	timing, _ := animationmodel.NewTiming(time.Second, 0, animationmodel.Linear{})
	animations := make([]CSSAnimation, MaxAnimationsPerElement+20)
	for index := range animations {
		animations[index] = CSSAnimation{Name: "pulse", Timing: timing, Iterations: math.Inf(1)}
	}
	if got := NewAnimationStack(animations, time.Unix(100, 0)).Len(); got != MaxAnimationsPerElement {
		t.Fatalf("element animation count = %d, want cap %d", got, MaxAnimationsPerElement)
	}

	elementCount := MaxActiveAnimations/MaxAnimationsPerElement + 2
	styles := make(Map, elementCount)
	for index := 0; index < elementCount; index++ {
		styles[dom.NodeID(index+1)] = ComputedStyle{Animations: animations}
	}
	registry := NewAnimationRegistry()
	registry.Reconcile(styles, time.Unix(100, 0))
	if registry.Total() != MaxActiveAnimations {
		t.Fatalf("page animation count = %d, want cap %d", registry.Total(), MaxActiveAnimations)
	}
}

func TestPathologicalInterpolationStaysFinite(t *testing.T) {
	from := []TransformFunction{{Kind: TransformMatrix, A: math.MaxFloat32, D: math.MaxFloat32}}
	to := []TransformFunction{{Kind: TransformMatrix, A: -math.MaxFloat32, D: -math.MaxFloat32}}
	for _, progress := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1e300} {
		if opacity := InterpolateOpacity(0, 1, progress); math.IsNaN(float64(opacity)) || math.IsInf(float64(opacity), 0) {
			t.Fatalf("opacity at %v = %v", progress, opacity)
		}
		color := InterpolateColor(0xff0000ff, 0x0000ffff, progress)
		_ = color
		matrix := ResolveTransform(InterpolateTransform(from, to, progress, 100, 100), 100, 100)
		for _, component := range []float32{matrix.A, matrix.B, matrix.C, matrix.D, matrix.E, matrix.F} {
			if math.IsNaN(float64(component)) || math.IsInf(float64(component), 0) {
				t.Fatalf("transform component at %v = %v in %#v", progress, component, matrix)
			}
		}
	}
}
