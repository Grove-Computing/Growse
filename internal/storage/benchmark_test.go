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
