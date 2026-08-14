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
	}, nil, false)
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
	api := newAPI(context.Background(), &fakeClock{}, func(func()) bool { return true }, nil, false)
	t.Cleanup(api.Close)

	if id, err := api.SetTimeout(time.Second, nil); err == nil || id != 0 {
		t.Fatalf("SetTimeout(nil) = (%d, %v), want zero ID and error", id, err)
	}
}

func TestClearTimerCancelsPendingTimeoutAndInterval(t *testing.T) {
	start := time.Unix(100, 0)
	clock := &fakeClock{current: start}
	callbackCount := 0
	api := newAPI(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, nil, false)
	t.Cleanup(api.Close)

	timeoutID, err := api.SetTimeout(time.Second, func() { callbackCount++ })
	if err != nil {
		t.Fatal(err)
	}
	intervalID, err := api.SetInterval(time.Second, func() { callbackCount++ })
	if err != nil {
		t.Fatal(err)
	}
	if !api.ClearTimer(timeoutID) || !api.ClearTimer(intervalID) {
		t.Fatal("ClearTimer() rejected active timers")
	}
	if api.ClearTimer(timeoutID) || api.ClearTimer(0) {
		t.Fatal("ClearTimer() accepted an inactive timer")
	}

	clock.current = start.Add(2 * time.Second)
	api.runDue(clock.Now())
	if callbackCount != 0 {
		t.Fatalf("callback count = %d, want 0", callbackCount)
	}
}

func TestIntervalCanClearItself(t *testing.T) {
	start := time.Unix(100, 0)
	clock := &fakeClock{current: start}
	callbackCount := 0
	api := newAPI(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, nil, false)
	t.Cleanup(api.Close)

	var intervalID TimerID
	intervalID, _ = api.SetInterval(time.Millisecond, func() {
		callbackCount++
		api.ClearTimer(intervalID)
	})
	clock.current = start.Add(time.Millisecond)
	api.runDue(clock.Now())
	clock.current = start.Add(2 * time.Millisecond)
	api.runDue(clock.Now())
	if callbackCount != 1 {
		t.Fatalf("callback count = %d, want 1", callbackCount)
	}
}

func TestSchedulerClampsRegressedClockAndKeepsRegistrationOrder(t *testing.T) {
	start := time.Unix(100, 0)
	clock := &fakeClock{current: start}
	var callbacks []int
	api := newAPI(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, nil, false)
	t.Cleanup(api.Close)

	if _, err := api.SetTimeout(10*time.Millisecond, func() { callbacks = append(callbacks, 1) }); err != nil {
		t.Fatal(err)
	}
	clock.current = start.Add(-time.Hour)
	if _, err := api.SetTimeout(10*time.Millisecond, func() { callbacks = append(callbacks, 2) }); err != nil {
		t.Fatal(err)
	}

	clock.current = start.Add(10 * time.Millisecond)
	api.runDue(clock.Now())
	if len(callbacks) != 2 || callbacks[0] != 1 || callbacks[1] != 2 {
		t.Fatalf("callbacks = %v, want [1 2]", callbacks)
	}
	if got := api.lastNow; !got.Equal(clock.current) {
		t.Fatalf("last observed time = %v, want %v", got, clock.current)
	}

	clock.current = start
	api.runDue(clock.Now())
	if got := api.lastNow; !got.Equal(start.Add(10 * time.Millisecond)) {
		t.Fatalf("regressed clock changed observed time to %v", got)
	}
}

func TestSchedulerNormalizesNegativeDelayAndRejectsExtremeDuration(t *testing.T) {
	start := time.Unix(100, 0)
	clock := &fakeClock{current: start}
	called := false
	api := newAPI(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, nil, false)
	t.Cleanup(api.Close)

	if _, err := api.SetTimeout(-time.Second, func() { called = true }); err != nil {
		t.Fatalf("SetTimeout(negative) error = %v", err)
	}
	api.runDue(start)
	if !called {
		t.Fatal("negative delay was not normalized to zero")
	}
	if id, err := api.SetTimeout(MaxTimerDuration+time.Nanosecond, func() {}); err == nil || id != 0 {
		t.Fatalf("SetTimeout(extreme) = (%d, %v), want zero ID and error", id, err)
	}
}

func TestSchedulerClampsDeeplyNestedTimers(t *testing.T) {
	start := time.Unix(100, 0)
	clock := &fakeClock{current: start}
	api := newAPI(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, nil, false)
	t.Cleanup(api.Close)

	callbackCount := 0
	var nested func()
	nested = func() {
		callbackCount++
		if callbackCount < 7 {
			if _, err := api.SetTimeout(0, nested); err != nil {
				t.Fatalf("nested SetTimeout() error = %v", err)
			}
		}
	}
	if _, err := api.SetTimeout(0, nested); err != nil {
		t.Fatal(err)
	}
	api.runDue(start)
	if callbackCount != 5 {
		t.Fatalf("callbacks before clamp = %d, want 5", callbackCount)
	}
	clock.current = start.Add(3 * time.Millisecond)
	api.runDue(clock.Now())
	if callbackCount != 5 {
		t.Fatalf("callback ran before 4ms clamp: %d", callbackCount)
	}
	clock.current = start.Add(4 * time.Millisecond)
	api.runDue(clock.Now())
	if callbackCount != 6 {
		t.Fatalf("callbacks after first clamp = %d, want 6", callbackCount)
	}
	clock.current = start.Add(8 * time.Millisecond)
	api.runDue(clock.Now())
	if callbackCount != 7 {
		t.Fatalf("callbacks after second clamp = %d, want 7", callbackCount)
	}
}

func TestDelayedIntervalSkipsMissedExecutions(t *testing.T) {
	start := time.Unix(100, 0)
	clock := &fakeClock{current: start}
	callbackCount := 0
	api := newAPI(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, nil, false)
	t.Cleanup(api.Close)

	intervalID, err := api.SetInterval(10*time.Millisecond, func() { callbackCount++ })
	if err != nil {
		t.Fatal(err)
	}
	clock.current = start.Add(time.Second)
	api.runDue(clock.Now())
	if callbackCount != 1 {
		t.Fatalf("delayed callback count = %d, want 1", callbackCount)
	}
	api.mu.Lock()
	nextDeadline := api.timers[intervalID].deadline
	api.mu.Unlock()
	if want := clock.current.Add(10 * time.Millisecond); !nextDeadline.Equal(want) {
		t.Fatalf("next deadline = %v, want %v", nextDeadline, want)
	}
	api.runDue(clock.Now())
	if callbackCount != 1 {
		t.Fatalf("missed intervals were delivered in a burst: %d", callbackCount)
	}
}

func TestAnimationFrameRegistersCancelsAndDefersNestedCallback(t *testing.T) {
	start := time.Unix(100, 0)
	clock := &fakeClock{current: start}
	requests := 0
	api := newAPI(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, func() {
		requests++
	}, false)
	t.Cleanup(api.Close)

	var timestamps []Timestamp
	firstID, err := api.RequestAnimationFrame(func(timestamp Timestamp) {
		timestamps = append(timestamps, timestamp)
		if _, nestedErr := api.RequestAnimationFrame(func(next Timestamp) {
			timestamps = append(timestamps, next)
		}); nestedErr != nil {
			t.Errorf("nested RequestAnimationFrame() error = %v", nestedErr)
		}
	})
	if err != nil || firstID == 0 {
		t.Fatalf("RequestAnimationFrame() = (%d, %v), want non-zero ID", firstID, err)
	}
	canceledID, err := api.RequestAnimationFrame(func(Timestamp) {
		t.Fatal("canceled frame callback was executed")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !api.CancelAnimationFrame(canceledID) || api.CancelAnimationFrame(canceledID) {
		t.Fatal("CancelAnimationFrame() did not report active state")
	}
	if requests != 2 {
		t.Fatalf("frame requests = %d, want 2", requests)
	}

	firstFrame := start.Add(16 * time.Millisecond)
	if !api.RunAnimationFrame(firstFrame) {
		t.Fatal("first frame did not run")
	}
	if len(timestamps) != 1 || timestamps[0] != Timestamp(16*time.Millisecond) {
		t.Fatalf("first frame timestamps = %v", timestamps)
	}
	if !api.HasAnimationFrameCallbacks() {
		t.Fatal("callback registered during a frame was not deferred")
	}

	secondFrame := start.Add(32 * time.Millisecond)
	api.RunAnimationFrame(secondFrame)
	if len(timestamps) != 2 || timestamps[1] != Timestamp(32*time.Millisecond) {
		t.Fatalf("second frame timestamps = %v", timestamps)
	}
	if api.HasAnimationFrameCallbacks() {
		t.Fatal("one-shot frame callback remained active")
	}
}

func TestCloseDiscardsTimerAndFrameCallbacks(t *testing.T) {
	start := time.Unix(100, 0)
	clock := &fakeClock{current: start}
	callbackCount := 0
	api := newAPI(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, func() {}, false)
	timerID, err := api.SetTimeout(time.Second, func() { callbackCount++ })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.RequestAnimationFrame(func(Timestamp) { callbackCount++ }); err != nil {
		t.Fatal(err)
	}

	api.Close()
	clock.current = start.Add(2 * time.Second)
	api.runDue(clock.Now())
	if api.RunAnimationFrame(clock.Now()) || api.HasAnimationFrameCallbacks() {
		t.Fatal("closed scheduler retained an animation frame callback")
	}
	if api.ClearTimer(timerID) {
		t.Fatal("closed scheduler retained a timer")
	}
	if callbackCount != 0 {
		t.Fatalf("callback count after Close = %d, want 0", callbackCount)
	}
}
