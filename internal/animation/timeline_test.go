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
	timeline := NewTimeline(clock)
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
