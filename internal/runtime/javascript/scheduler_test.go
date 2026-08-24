package javascript

import (
	"context"
	"testing"
	"time"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/dop251/goja"
)

type javascriptFakeClock struct{ current time.Time }

func (clock *javascriptFakeClock) Now() time.Time { return clock.current }

func TestSchedulerUsesPageQueueAndExistingTimingPolicy(t *testing.T) {
	start := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
	clock := &javascriptFakeClock{current: start}
	runtime := New()
	runtime.schedulerClock = clock
	t.Cleanup(func() { _ = runtime.Stop() })
	var records [][2]string
	frames := 0
	environment := runtimemodel.Environment{
		RequestFrame: func() { frames++ },
		ConsoleRecord: func(level, message string) {
			records = append(records, [2]string{level, message})
		},
	}
	source := `
		var calls = [];
		var canceled = setTimeout(function () { calls.push("canceled"); }, 1);
		clearTimeout(canceled);
		setTimeout(function (first, second) { calls.push(first + second); }, 10, "timeout", 1);
		setTimeout(function () { calls.push("timeout2"); }, 10);
		var interval = setInterval(function () { calls.push("interval"); clearInterval(interval); }, 5);
		var stringRejected = false;
		try { setTimeout("not code", 0); } catch (error) { stringRejected = true; }
		requestAnimationFrame(function (timestamp) { calls.push("frame:" + timestamp); });
		var canceledFrame = requestAnimationFrame(function () { calls.push("canceled frame"); });
		cancelAnimationFrame(canceledFrame);`
	startJavaScriptRuntime(t, runtime, source, environment)
	if frames != 2 || !runtime.HasAnimationFrameCallbacks() {
		t.Fatalf("frame requests = %d, pending = %v; want 2 and pending", frames, runtime.HasAnimationFrameCallbacks())
	}
	if !runtime.RunAnimationFrame(start.Add(16*time.Millisecond)) || runtime.HasAnimationFrameCallbacks() {
		t.Fatal("RunAnimationFrame() did not deliver and clear the pending frame")
	}

	runtime.SetBackground(true)
	clock.current = start.Add(20 * time.Millisecond)
	runtime.schedulerAPI.RunDue(clock.current)
	if err := runtime.runSync(context.Background(), func(*goja.Runtime) error { return nil }); err != nil {
		t.Fatalf("Page queue barrier: %v", err)
	}
	clock.current = start.Add(2 * time.Second)
	runtime.schedulerAPI.RunDue(clock.current)
	if err := runtime.runSync(context.Background(), func(*goja.Runtime) error { return nil }); err != nil {
		t.Fatalf("Page queue barrier: %v", err)
	}

	var calls []string
	var stringRejected bool
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		if err := vm.ExportTo(vm.Get("calls"), &calls); err != nil {
			return err
		}
		stringRejected = vm.Get("stringRejected").ToBoolean()
		return nil
	}); err != nil {
		t.Fatalf("read Scheduler results: %v", err)
	}
	want := []string{"frame:16", "timeout1", "timeout2", "interval"}
	if len(calls) != len(want) {
		t.Fatalf("Scheduler calls = %v, want %v", calls, want)
	}
	for index := range want {
		if calls[index] != want[index] {
			t.Fatalf("Scheduler calls = %v, want %v", calls, want)
		}
	}
	if !stringRejected {
		t.Fatal("setTimeout accepted a string callback")
	}
	if len(records) != 0 {
		t.Fatalf("unexpected Scheduler errors = %v", records)
	}
}

func TestSchedulerCloseDropsPendingCallbacks(t *testing.T) {
	start := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
	clock := &javascriptFakeClock{current: start}
	runtime := New()
	runtime.schedulerClock = clock
	var messages []string
	environment := runtimemodel.Environment{ConsoleLog: func(message string) { messages = append(messages, message) }}
	startJavaScriptRuntime(t, runtime, `setTimeout(function () { console.log("late"); }, 100);`, environment)
	scheduler := runtime.schedulerAPI
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	clock.current = start.Add(time.Second)
	scheduler.RunDue(clock.current)
	if len(messages) != 0 {
		t.Fatalf("messages after close = %v, want none", messages)
	}
}
