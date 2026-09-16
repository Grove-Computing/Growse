package style

import (
	"math"
	"sync"
	"testing"
	"time"

	animationmodel "github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestAnimationRegistrySupportsConcurrentReconcileAndSampling(t *testing.T) {
	t.Parallel()
	start := time.Unix(100, 0)
	timing, err := animationmodel.NewTiming(time.Second, 0, animationmodel.Linear{})
	if err != nil {
		t.Fatal(err)
	}
	nodeID := dom.NodeID(7)
	styles := Map{nodeID: {Animations: []CSSAnimation{{
		Name: "pulse", Timing: timing, Iterations: math.Inf(1), PlayState: AnimationRunning,
	}}}}
	registry := NewAnimationRegistry()

	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(offset int) {
			defer workers.Done()
			for iteration := 0; iteration < 200; iteration++ {
				current := start.Add(time.Duration(offset*200+iteration) * time.Millisecond)
				if offset%2 == 0 {
					registry.Reconcile(styles, current)
					if iteration%11 == 0 {
						registry.Clear()
					}
					continue
				}
				_ = registry.Sample(nodeID, current)
				_ = registry.Count(nodeID)
				_ = registry.Total()
				_ = registry.ActiveNodes(current)
				_ = registry.Active(current)
			}
		}(worker)
	}
	workers.Wait()
}

func TestTransitionRegistrySupportsConcurrentReconcileAndSampling(t *testing.T) {
	t.Parallel()
	start := time.Unix(100, 0)
	timing, err := animationmodel.NewTiming(time.Second, 0, animationmodel.Linear{})
	if err != nil {
		t.Fatal(err)
	}
	nodeID := dom.NodeID(42)
	previous := Map{nodeID: {Opacity: 0}}
	next := Map{nodeID: {Opacity: 1, Transitions: []Transition{{Property: "opacity", Timing: timing}}}}
	registry := NewTransitionRegistry()

	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(offset int) {
			defer workers.Done()
			for iteration := 0; iteration < 200; iteration++ {
				current := start.Add(time.Duration(offset*200+iteration) * time.Millisecond)
				if offset%2 == 0 {
					registry.Reconcile(previous, next, current)
					if iteration%11 == 0 {
						registry.Clear()
					}
					continue
				}
				_ = registry.Apply(next, current)
				_ = registry.Active(current)
				_ = registry.Count(nodeID)
				_ = registry.ActiveNodes(current)
			}
		}(worker)
	}
	workers.Wait()
}
