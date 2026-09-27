package omnibox

import (
	"fmt"
	"testing"
	"time"
)

func BenchmarkRankHistorySnapshot(b *testing.B) {
	snapshot := Snapshot{Now: time.Unix(1_700_000_000, 0)}
	for i := 0; i < 50_000; i++ {
		snapshot.History = append(snapshot.History, Candidate{Primary: "Growse documentation", URL: fmt.Sprintf("https://example.com/docs/%d", i), LastVisited: snapshot.Now.Add(-time.Duration(i) * time.Hour), VisitCount: uint32(i % 100)})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Rank("growse", snapshot, nil)
	}
}
