package scheduler

import (
	"context"
	"testing"
	"time"
)

type benchmarkClock struct{ current time.Time }

func (clock benchmarkClock) Now() time.Time { return clock.current }

func BenchmarkRegisterAndClear10000Timers(b *testing.B) {
	clock := benchmarkClock{current: time.Unix(100, 0)}
	b.ReportAllocs()
	b.ReportMetric(10000, "timers/op")
	b.ResetTimer()
	for b.Loop() {
		api := NewPageWithClock(context.Background(), clock, func(func()) bool { return true }, nil)
		ids := make([]TimerID, 10000)
		for index := range ids {
			id, err := api.SetTimeout(time.Hour, func() {})
			if err != nil {
				b.Fatal(err)
			}
			ids[index] = id
		}
		for _, id := range ids {
			if !api.ClearTimer(id) {
				b.Fatalf("ClearTimer(%d) failed", id)
			}
		}
		api.Close()
	}
}
