package style

import (
	"math"
	"testing"
	"time"

	animationmodel "github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/dom"
)

// BenchmarkSample100ElementAnimations measures one frame of timing work for
// 100 independently animated elements at a shared timestamp.
func BenchmarkSample100ElementAnimations(b *testing.B) {
	start := time.Unix(100, 0)
	timing, err := animationmodel.NewTiming(time.Second, 0, animationmodel.Linear{})
	if err != nil {
		b.Fatal(err)
	}
	styles := make(Map, 100)
	nodeIDs := make([]dom.NodeID, 100)
	for index := range nodeIDs {
		nodeID := dom.NodeID(index + 1)
		nodeIDs[index] = nodeID
		styles[nodeID] = ComputedStyle{Animations: []CSSAnimation{{
			Name: "pulse", Timing: timing, Iterations: math.Inf(1), PlayState: AnimationRunning,
		}}}
	}
	registry := NewAnimationRegistry()
	registry.Reconcile(styles, start)
	frameTime := start.Add(375 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()
	b.ReportMetric(100, "animations/op")
	for b.Loop() {
		for _, nodeID := range nodeIDs {
			_ = registry.Sample(nodeID, frameTime)
		}
	}
}
