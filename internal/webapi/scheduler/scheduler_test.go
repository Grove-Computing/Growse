package scheduler

import (
	"context"
	"testing"
	"time"
)

type fakeClock struct {
	current time.Time
}

func (clock *fakeClock) Now() time.Time {
	return clock.current
}

func TestTimeoutAndIntervalRegisterCallbacks(t *testing.T) {
	start := time.Unix(100, 0)
	clock := &fakeClock{current: start}
	var callbacks []string
	api := newAPI(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, false)
	t.Cleanup(api.Close)

	timeoutID, err := api.SetTimeout(10*time.Millisecond, func() {
		callbacks = append(callbacks, "timeout")
	})
	if err != nil || timeoutID == 0 {
		t.Fatalf("SetTimeout() = (%d, %v), want non-zero ID", timeoutID, err)
	}
	intervalID, err := api.SetInterval(5*time.Millisecond, func() {
		callbacks = append(callbacks, "interval")
	})
	if err != nil || intervalID == 0 || intervalID == timeoutID {
		t.Fatalf("SetInterval() = (%d, %v), want distinct non-zero ID", intervalID, err)
	}

	clock.current = start.Add(5 * time.Millisecond)
	api.runDue(clock.Now())
	clock.current = start.Add(10 * time.Millisecond)
	api.runDue(clock.Now())

	want := []string{"interval", "timeout", "interval"}
	if len(callbacks) != len(want) {
		t.Fatalf("callbacks = %v, want %v", callbacks, want)
	}
	for index := range want {
		if callbacks[index] != want[index] {
			t.Fatalf("callbacks = %v, want %v", callbacks, want)
		}
	}
}

func TestSchedulerRejectsMissingCallback(t *testing.T) {
	api := newAPI(context.Background(), &fakeClock{}, func(func()) bool { return true }, false)
	t.Cleanup(api.Close)

	if id, err := api.SetTimeout(time.Second, nil); err == nil || id != 0 {
		t.Fatalf("SetTimeout(nil) = (%d, %v), want zero ID and error", id, err)
	}
}
