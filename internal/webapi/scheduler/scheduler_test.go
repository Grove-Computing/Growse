package scheduler

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
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

func TestSchedulerBoundsPendingTimerAndFrameCallbacks(t *testing.T) {
	api := newAPI(context.Background(), &fakeClock{}, func(func()) bool { return true }, nil, false)
	t.Cleanup(api.Close)
	timerIDs := make([]TimerID, MaxTimersPerPage)
	for index := range timerIDs {
		id, err := api.SetTimeout(time.Hour, func() {})
		if err != nil {
			t.Fatalf("SetTimeout(%d) error = %v", index, err)
		}
		timerIDs[index] = id
	}
	if id, err := api.SetTimeout(time.Hour, func() {}); err == nil || id != 0 {
		t.Fatalf("SetTimeout beyond limit = (%d, %v)", id, err)
	}
	if !api.ClearTimer(timerIDs[0]) {
		t.Fatal("ClearTimer() did not release one limit slot")
	}
	if id, err := api.SetTimeout(time.Hour, func() {}); err != nil || id == 0 {
		t.Fatalf("SetTimeout after clear = (%d, %v)", id, err)
	}

	frameIDs := make([]FrameID, MaxFrameCallbacksPerPage)
	for index := range frameIDs {
		id, err := api.RequestAnimationFrame(func(Timestamp) {})
		if err != nil {
			t.Fatalf("RequestAnimationFrame(%d) error = %v", index, err)
		}
		frameIDs[index] = id
	}
	if id, err := api.RequestAnimationFrame(func(Timestamp) {}); err == nil || id != 0 {
		t.Fatalf("RequestAnimationFrame beyond limit = (%d, %v)", id, err)
	}
	if !api.CancelAnimationFrame(frameIDs[0]) {
		t.Fatal("CancelAnimationFrame() did not release one limit slot")
	}
	if id, err := api.RequestAnimationFrame(func(Timestamp) {}); err != nil || id == 0 {
		t.Fatalf("RequestAnimationFrame after cancel = (%d, %v)", id, err)
	}
}

func TestSchedulerBoundsCallbacksDeliveredPerTurn(t *testing.T) {
	clock := &fakeClock{current: time.Unix(100, 0)}
	delivered := 0
	api := newAPI(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, nil, false)
	t.Cleanup(api.Close)
	for index := 0; index < MaxCallbacksPerTurn+1; index++ {
		if _, err := api.SetTimeout(0, func() { delivered++ }); err != nil {
			t.Fatal(err)
		}
	}
	api.runDue(clock.Now())
	if delivered != MaxCallbacksPerTurn {
		t.Fatalf("first turn callbacks = %d, want %d", delivered, MaxCallbacksPerTurn)
	}
	api.runDue(clock.Now())
	if delivered != MaxCallbacksPerTurn+1 {
		t.Fatalf("second turn callbacks = %d, want %d", delivered, MaxCallbacksPerTurn+1)
	}
}

func TestSchedulerBudgetsAnimationFrameCallbacksAcrossTicks(t *testing.T) {
	start := time.Unix(100, 0)
	clock := &fakeClock{current: start}
	frameRequests := 0
	api := newAPI(context.Background(), clock, func(func()) bool { return true }, func() { frameRequests++ }, false)
	t.Cleanup(api.Close)
	delivered := make([]int, 0, MaxFrameCallbacksPerTick+3)
	for index := 0; index < MaxFrameCallbacksPerTick+3; index++ {
		value := index
		if _, err := api.RequestAnimationFrame(func(Timestamp) { delivered = append(delivered, value) }); err != nil {
			t.Fatal(err)
		}
	}
	if !api.RunAnimationFrame(start.Add(16 * time.Millisecond)) {
		t.Fatal("first budgeted frame did not run")
	}
	if len(delivered) != MaxFrameCallbacksPerTick || !api.HasAnimationFrameCallbacks() {
		t.Fatalf("first frame delivered=%d pending=%t", len(delivered), api.HasAnimationFrameCallbacks())
	}
	if !api.RunAnimationFrame(start.Add(32*time.Millisecond)) || api.HasAnimationFrameCallbacks() {
		t.Fatal("overflow callbacks were not drained on the next frame")
	}
	if len(delivered) != MaxFrameCallbacksPerTick+3 {
		t.Fatalf("total delivered callbacks = %d", len(delivered))
	}
	for index, value := range delivered {
		if value != index {
			t.Fatalf("callback order[%d] = %d", index, value)
		}
	}
	if frameRequests < MaxFrameCallbacksPerTick+4 {
		t.Fatalf("frame requests = %d, overflow did not request another frame", frameRequests)
	}
}

func TestSchedulerRecoversCallbackPanicsWithoutLoggingPayload(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	clock := &fakeClock{current: time.Unix(100, 0)}
	api := newAPI(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, nil, false)
	t.Cleanup(api.Close)
	delivered := 0
	_, _ = api.SetTimeout(0, func() { panic("storage-value-secret") })
	_, _ = api.SetTimeout(0, func() { delivered++ })
	api.runDue(clock.Now())
	_, _ = api.RequestAnimationFrame(func(Timestamp) { panic("credential-secret") })
	_, _ = api.RequestAnimationFrame(func(Timestamp) { delivered++ })
	api.RunAnimationFrame(clock.Now())
	if delivered != 2 {
		t.Fatalf("callbacks after panic = %d, want 2", delivered)
	}
	message := logs.String()
	if !strings.Contains(message, "component=scheduler") || !strings.Contains(message, "type=timer") || !strings.Contains(message, "type=frame") {
		t.Fatalf("generic Scheduler panic logs = %q", message)
	}
	for _, secret := range []string{"storage-value-secret", "credential-secret"} {
		if strings.Contains(message, secret) {
			t.Fatalf("Scheduler panic log exposed %q: %s", secret, message)
		}
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

func TestBackgroundTimersUseOneSecondClampAndCallbackBudget(t *testing.T) {
	clock := &fakeClock{current: time.Unix(500, 0)}
	fired := 0
	api := newAPI(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, nil, false)
	api.SetBackground(true)
	for index := 0; index < MaxBackgroundCallbacksPerTurn+50; index++ {
		if _, err := api.SetTimeout(0, func() { fired++ }); err != nil {
			t.Fatal(err)
		}
	}
	api.RunDue(clock.current.Add(MinBackgroundTimerDelay - time.Millisecond))
	if fired != 0 {
		t.Fatalf("background timer fired before clamp: %d", fired)
	}
	api.RunDue(clock.current.Add(MinBackgroundTimerDelay))
	if fired != MaxBackgroundCallbacksPerTurn {
		t.Fatalf("callbacks in first background turn = %d, want %d", fired, MaxBackgroundCallbacksPerTurn)
	}
	api.RunDue(clock.current.Add(MinBackgroundTimerDelay))
	if fired != MaxBackgroundCallbacksPerTurn+50 {
		t.Fatalf("callbacks after second background turn = %d, want %d", fired, MaxBackgroundCallbacksPerTurn+50)
	}
}
