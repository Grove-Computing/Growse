package style

import (
	"math"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/animation"
)

func TestInterpolateTransformListAndIdentityPadding(t *testing.T) {
	from := []TransformFunction{{
		Kind: TransformTranslate,
		X:    LengthPercentage{Pixels: 10, Percentage: 20},
	}}
	to := []TransformFunction{
		{Kind: TransformTranslate, X: LengthPercentage{Pixels: 30, Percentage: 60}, Y: LengthPercentage{Pixels: 20}},
		{Kind: TransformScale, A: 3, D: 5},
	}
	got := InterpolateTransform(from, to, 0.5, 200, 100)
	if len(got) != 2 {
		t.Fatalf("transform list length = %d, want 2", len(got))
	}
	if got[0].Kind != TransformTranslate || got[0].X != (LengthPercentage{Pixels: 20, Percentage: 40}) || got[0].Y.Pixels != 10 {
		t.Fatalf("interpolated translate = %#v", got[0])
	}
	if got[1].Kind != TransformScale || got[1].A != 2 || got[1].D != 3 {
		t.Fatalf("identity-padded scale = %#v", got[1])
	}
}

func TestInterpolateTransformFallsBackToDecomposedMatrix(t *testing.T) {
	from := []TransformFunction{{Kind: TransformTranslate, X: LengthPercentage{Percentage: 50}}}
	to := []TransformFunction{{Kind: TransformScale, A: 3, D: 3}}
	got := InterpolateTransform(from, to, 0.5, 200, 100)
	if len(got) != 1 || got[0].Kind != TransformMatrix {
		t.Fatalf("fallback transform = %#v, want one matrix", got)
	}
	matrix := ResolveTransform(got, 200, 100)
	assertMatrixNear(t, matrix, Matrix{A: 2, D: 2, E: 50})
}

func TestInterpolateTransformMatrixUsesRotationDecomposition(t *testing.T) {
	from := []TransformFunction{{Kind: TransformMatrix, A: 1, D: 1}}
	to := []TransformFunction{{Kind: TransformMatrix, B: 1, C: -1, E: 100}}
	got := InterpolateTransform(from, to, 0.5, 0, 0)
	wantRoot := float32(math.Sqrt(0.5))
	assertMatrixNear(t, ResolveTransform(got, 0, 0), Matrix{
		A: wantRoot, B: wantRoot, C: -wantRoot, D: wantRoot, E: 50,
	})
}

func TestRunningTransitionInterpolatesTransform(t *testing.T) {
	start := time.Unix(100, 0)
	timing, err := animation.NewTiming(time.Second, 0, animation.Linear{})
	if err != nil {
		t.Fatal(err)
	}
	running := NewRunningTransition(StartedTransition{
		Property: "transform",
		From: TransitionValue{Kind: TransitionTransform, Transform: []TransformFunction{{
			Kind: TransformTranslate, X: LengthPercentage{Pixels: 10},
		}}},
		To: TransitionValue{Kind: TransitionTransform, Transform: []TransformFunction{{
			Kind: TransformTranslate, X: LengthPercentage{Pixels: 30},
		}}},
		Timing: timing,
	}, start)
	value, active := running.Advance(start.Add(timing.Duration / 2))
	if !active || len(value.Transform) != 1 || value.Transform[0].X.Pixels != 20 {
		t.Fatalf("transform midpoint = (%#v, %v)", value.Transform, active)
	}
}

func assertMatrixNear(t *testing.T, got, want Matrix) {
	t.Helper()
	left := []float32{got.A, got.B, got.C, got.D, got.E, got.F}
	right := []float32{want.A, want.B, want.C, want.D, want.E, want.F}
	for index := range left {
		if abs32(left[index]-right[index]) > 0.00001 {
			t.Fatalf("matrix = %#v, want %#v", got, want)
		}
	}
}
