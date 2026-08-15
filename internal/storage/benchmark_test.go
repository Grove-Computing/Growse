package storage

import (
	"fmt"
	"testing"
)

func BenchmarkLookupAndUpdate10000StorageEntries(b *testing.B) {
	area := NewArea()
	keys := make([]string, 10000)
	for index := range keys {
		keys[index] = fmt.Sprintf("note-%05d", index)
		if err := area.Set(keys[index], "value"); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ReportMetric(10000, "entries/op")
	b.ResetTimer()
	for iteration := 0; b.Loop(); iteration++ {
		for _, key := range keys {
			if _, ok := area.Get(key); !ok {
				b.Fatalf("missing key %s", key)
			}
		}
		value := "even"
		if iteration%2 != 0 {
			value = "odd"
		}
		if err := area.Set(keys[iteration%len(keys)], value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDispatchLocalStorageUpdatesAcross16Tabs(b *testing.B) {
	area := NewArea()
	unsubscribes := make([]func(), 16)
	for index := range unsubscribes {
		unsubscribes[index] = area.Subscribe(uint64(index+1), func(Change) {})
	}
	defer func() {
		for _, unsubscribe := range unsubscribes {
			unsubscribe()
		}
	}()
	b.ReportAllocs()
	b.ReportMetric(16, "updates/op")
	iteration := 0
	b.ResetTimer()
	for b.Loop() {
		for source := 0; source < 16; source++ {
			value := fmt.Sprintf("%d-%d", iteration, source)
			if err := area.SetFrom(MutationSource{ID: uint64(source + 1)}, "shared", value); err != nil {
				b.Fatal(err)
			}
		}
		iteration++
	}
}
