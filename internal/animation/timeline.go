// Package animation provides the time model shared by CSS transitions and animations.
package animation

import "time"

// Clock supplies the current animation time. Implementations must prefer a
// monotonic time source; Timeline additionally prevents observed time from
// moving backwards.
type Clock interface {
	Now() time.Time
}

// Animation advances one animation to a frame timestamp. It reports whether
// it remains active after the frame.
type Animation interface {
	Advance(time.Time) bool
}

// Timeline advances every registered animation against one shared clock.
type Timeline struct {
	clock        Clock
	requestFrame func()
	lastFrame    time.Time
	active       []Animation
	framePending bool
}

// NewTimeline creates an empty timeline driven by clock.
func NewTimeline(clock Clock, requestFrame func()) *Timeline {
	if clock == nil {
		clock = systemClock{}
	}
	return &Timeline{clock: clock, requestFrame: requestFrame}
}

// Add registers an animation for the next frame. Nil animations are ignored.
func (t *Timeline) Add(animation Animation) {
	if animation != nil {
		t.active = append(t.active, animation)
		t.requestNextFrame()
	}
}

// Active reports whether at least one animation needs another frame.
func (t *Timeline) Active() bool {
	return len(t.active) > 0
}

// Tick samples the clock once and advances all active animations with the
// resulting timestamp. Finished animations are removed from the timeline.
func (t *Timeline) Tick() time.Time {
	t.framePending = false
	frameTime := t.clock.Now()
	if !t.lastFrame.IsZero() && frameTime.Before(t.lastFrame) {
		frameTime = t.lastFrame
	}
	t.lastFrame = frameTime

	remaining := t.active[:0]
	for _, animation := range t.active {
		if animation.Advance(frameTime) {
			remaining = append(remaining, animation)
		}
	}
	t.active = remaining
	if len(t.active) > 0 {
		t.requestNextFrame()
	}
	return frameTime
}

func (t *Timeline) requestNextFrame() {
	if t.requestFrame == nil || t.framePending {
		return
	}
	t.framePending = true
	t.requestFrame()
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}
