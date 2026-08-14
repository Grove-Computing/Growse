package animation

import (
	"testing"
	"time"
)

func TestTimelineUsesOneMonotonicTimestampForEveryAnimation(t *testing.T) {
	start := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	clock := &sequenceClock{times: []time.Time{
		start.Add(2 * time.Second),
		start.Add(time.Second),
	}}
	first := &recordingAnimation{activeFrames: 2}
	second := &recordingAnimation{activeFrames: 2}
	timeline := NewTimeline(clock, nil)
	timeline.Add(first)
	timeline.Add(second)

	if got, want := timeline.Tick(), start.Add(2*time.Second); !got.Equal(want) {
		t.Fatalf("first frame time = %v, want %v", got, want)
	}
	if got, want := timeline.Tick(), start.Add(2*time.Second); !got.Equal(want) {
		t.Fatalf("clamped frame time = %v, want %v", got, want)
	}

	if clock.calls != 2 {
		t.Fatalf("clock calls = %d, want one call per frame", clock.calls)
	}
	for name, frames := range map[string][]time.Time{"first": first.frames, "second": second.frames} {
		if len(frames) != 2 {
			t.Fatalf("%s animation frames = %d, want 2", name, len(frames))
		}
		for index, frame := range frames {
			if !frame.Equal(start.Add(2 * time.Second)) {
				t.Fatalf("%s animation frame %d = %v, want shared monotonic timestamp", name, index, frame)
			}
		}
	}
}

func TestTimelineRequestsFramesOnlyWhileAnimationIsActive(t *testing.T) {
	clock := &sequenceClock{times: []time.Time{
		time.Unix(1, 0),
		time.Unix(2, 0),
		time.Unix(3, 0),
	}}
	requests := 0
	timeline := NewTimeline(clock, func() { requests++ })

	timeline.Add(&recordingAnimation{activeFrames: 2})
	timeline.Add(&recordingAnimation{activeFrames: 1})
	if requests != 1 {
		t.Fatalf("frame requests after two additions = %d, want 1", requests)
	}
	if !timeline.Active() {
		t.Fatal("timeline is inactive after adding animations")
	}

	timeline.Tick()
	if requests != 2 {
		t.Fatalf("frame requests after active tick = %d, want 2", requests)
	}
	if !timeline.Active() {
		t.Fatal("timeline became inactive before the remaining animation finished")
	}

	timeline.Tick()
	if requests != 2 {
		t.Fatalf("frame requests after final tick = %d, want no additional request", requests)
	}
	if timeline.Active() {
		t.Fatal("timeline remains active after every animation finished")
	}

	timeline.Tick()
	if requests != 2 {
		t.Fatalf("frame requests after empty tick = %d, want no additional request", requests)
	}
}

type sequenceClock struct {
	times []time.Time
	calls int
}

func (clock *sequenceClock) Now() time.Time {
	index := clock.calls
	clock.calls++
	return clock.times[index]
}

type recordingAnimation struct {
	activeFrames int
	frames       []time.Time
}

func (animation *recordingAnimation) Advance(frameTime time.Time) bool {
	animation.frames = append(animation.frames, frameTime)
	return len(animation.frames) < animation.activeFrames
}
