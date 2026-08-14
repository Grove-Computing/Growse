package style

import (
	"reflect"
	"time"

	animationmodel "github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/dom"
)

type transitionKey struct {
	nodeID   dom.NodeID
	property string
}

// TransitionRegistry owns the transitions started by computed-style changes
// on one page. Author styles remain immutable while sampled values are
// composited at frame time.
type TransitionRegistry struct {
	items map[transitionKey]*RunningTransition
}

// NewTransitionRegistry creates an empty page transition registry.
func NewTransitionRegistry() *TransitionRegistry {
	return &TransitionRegistry{items: make(map[transitionKey]*RunningTransition)}
}

// Reconcile starts, interrupts, or cancels transitions after style
// recalculation. Initial style calculation must not call this method with a
// nil previous map.
func (registry *TransitionRegistry) Reconcile(previous, next Map, current time.Time) {
	if registry == nil || previous == nil {
		return
	}
	if registry.items == nil {
		registry.items = make(map[transitionKey]*RunningTransition)
	}

	for key, running := range registry.items {
		nextStyle, exists := next[key.nodeID]
		if !exists {
			delete(registry.items, key)
			continue
		}
		timing, enabled := transitionTimingForProperty(nextStyle.Transitions, key.property)
		target, supported := computedTransitionValue(nextStyle, key.property)
		if !enabled || timing.Duration <= 0 || !supported {
			delete(registry.items, key)
			continue
		}
		if !reflect.DeepEqual(target, running.To) {
			running.Interrupt(current, target, timing)
		}
	}

	for _, started := range StartTransitions(previous, next) {
		key := transitionKey{nodeID: started.NodeID, property: started.Property}
		if _, exists := registry.items[key]; exists {
			continue
		}
		if len(registry.items) >= MaxActiveAnimations {
			break
		}
		registry.items[key] = NewRunningTransition(started, current)
	}
}

// Apply samples transitions and composites them after CSS Animations.
func (registry *TransitionRegistry) Apply(styles Map, current time.Time) Map {
	result := make(Map, len(styles))
	for nodeID, computed := range styles {
		result[nodeID] = computed
	}
	if registry == nil {
		return result
	}

	valuesByNode := make(map[dom.NodeID]AnimatedValues)
	for key, running := range registry.items {
		if _, exists := result[key.nodeID]; !exists {
			delete(registry.items, key)
			continue
		}
		value, active := running.Advance(current)
		if !active {
			delete(registry.items, key)
			continue
		}
		if valuesByNode[key.nodeID] == nil {
			valuesByNode[key.nodeID] = AnimatedValues{}
		}
		valuesByNode[key.nodeID][key.property] = value
	}
	for nodeID, values := range valuesByNode {
		result[nodeID] = ApplyAnimatedCascade(result[nodeID], nil, values)
	}
	return result
}

// Active reports whether a transition can change on a future frame.
func (registry *TransitionRegistry) Active(current time.Time) bool {
	if registry == nil {
		return false
	}
	for _, running := range registry.items {
		if running.Timing.Sample(running.StartTime, current).Phase != animationmodel.PhaseAfter {
			return true
		}
	}
	return false
}

// Count reports the transitions retained for an element.
func (registry *TransitionRegistry) Count(nodeID dom.NodeID) int {
	if registry == nil {
		return 0
	}
	count := 0
	for key := range registry.items {
		if key.nodeID == nodeID {
			count++
		}
	}
	return count
}

// Clear discards every running transition owned by the page.
func (registry *TransitionRegistry) Clear() {
	if registry != nil {
		registry.items = make(map[transitionKey]*RunningTransition)
	}
}
