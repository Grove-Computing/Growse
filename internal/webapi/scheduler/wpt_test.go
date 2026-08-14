package scheduler

import (
	"context"
	"testing"
	"time"
)

// WPT source: html/webappapis/timers/negative-settimeout.any.js.
func TestWPTNegativeTimeoutRunsBeforePositiveTimeout(t *testing.T) {
	clock := &fakeClock{current: time.Unix(100, 0)}
	callbacks := make([]string, 0, 2)
	api := NewPageWithClock(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, nil)
	t.Cleanup(api.Close)
	if _, err := api.SetTimeout(-100*time.Millisecond, func() { callbacks = append(callbacks, "negative") }); err != nil {
		t.Fatal(err)
	}
	if _, err := api.SetTimeout(10*time.Millisecond, func() { callbacks = append(callbacks, "positive") }); err != nil {
		t.Fatal(err)
	}
	api.RunDue(clock.Now())
	if len(callbacks) != 1 || callbacks[0] != "negative" {
		t.Fatalf("callbacks at zero-delay deadline = %v", callbacks)
	}
}
