package style

import (
	"testing"
	"time"

	animationmodel "github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestTransitionRegistryCompositesAndDiscardsFinishedTransition(t *testing.T) {
	timing, err := animationmodel.NewTiming(time.Second, 0, animationmodel.Linear{})
	if err != nil {
		t.Fatal(err)
	}
	nodeID := dom.NodeID(42)
	previous := Map{nodeID: {Opacity: 0}}
	next := Map{nodeID: {Opacity: 1, Transitions: []Transition{{Property: "opacity", Timing: timing}}}}
	start := time.Unix(100, 0)
	registry := NewTransitionRegistry()
	registry.Reconcile(previous, next, start)

	if registry.Count(nodeID) != 1 || !registry.Active(start.Add(500*time.Millisecond)) {
		t.Fatalf("started transition = count:%d active:%v", registry.Count(nodeID), registry.Active(start.Add(500*time.Millisecond)))
	}
	if got := registry.Apply(next, start.Add(500*time.Millisecond))[nodeID].Opacity; got != 0.5 {
		t.Fatalf("midpoint opacity = %v, want 0.5", got)
	}
	if got := registry.Apply(next, start.Add(time.Second))[nodeID].Opacity; got != 1 {
		t.Fatalf("finished opacity = %v, want 1", got)
	}
	if registry.Count(nodeID) != 0 || registry.Active(start.Add(time.Second)) {
		t.Fatalf("finished transition retained = count:%d active:%v", registry.Count(nodeID), registry.Active(start.Add(time.Second)))
	}
}

func TestTransitionRegistryCancelsRemovedNode(t *testing.T) {
	timing, _ := animationmodel.NewTiming(time.Second, 0, animationmodel.Linear{})
	nodeID := dom.NodeID(7)
	registry := NewTransitionRegistry()
	registry.Reconcile(
		Map{nodeID: {Opacity: 0}},
		Map{nodeID: {Opacity: 1, Transitions: []Transition{{Property: "opacity", Timing: timing}}}},
		time.Unix(100, 0),
	)
	registry.Reconcile(Map{nodeID: {Opacity: 1}}, Map{}, time.Unix(100, 0))
	if registry.Count(nodeID) != 0 {
		t.Fatalf("removed node retained %d transitions", registry.Count(nodeID))
	}
}
