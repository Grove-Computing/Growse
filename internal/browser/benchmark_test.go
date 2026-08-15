package browser

import (
	"fmt"
	"net/url"
	"testing"
)

func BenchmarkTraverse1000HistoryEntries(b *testing.B) {
	fixture := newHistory()
	for index := 0; index < 1000; index++ {
		target, err := url.Parse(fmt.Sprintf("https://example.test/notes/%d", index))
		if err != nil {
			b.Fatal(err)
		}
		fixture.pushEntry(&historyEntry{URL: target, State: fmt.Sprintf(`{"index":%d}`, index), PageID: 1})
	}
	b.ReportAllocs()
	b.ReportMetric(1000, "entries/op")
	b.ResetTimer()
	for b.Loop() {
		working := fixture
		for working.canBack() {
			_, index, ok := working.targetEntry(-1)
			if !ok {
				b.Fatal("Back target missing")
			}
			working.index = index
		}
		for working.canForward() {
			_, index, ok := working.targetEntry(1)
			if !ok {
				b.Fatal("Forward target missing")
			}
			working.index = index
		}
	}
}

func BenchmarkCreateSwitchAndClose64Tabs(b *testing.B) {
	b.ReportAllocs()
	b.ReportMetric(64, "tabs/op")
	for b.Loop() {
		session := NewSession()
		ids := make([]TabID, 64)
		for index := range ids {
			tab, err := session.NewTab(nil)
			if err != nil {
				b.Fatal(err)
			}
			ids[index] = tab.ID
		}
		for _, id := range ids {
			if _, err := session.SelectTab(id); err != nil {
				b.Fatal(err)
			}
		}
		for index := len(ids) - 1; index > 0; index-- {
			if _, err := session.CloseTab(ids[index]); err != nil {
				b.Fatal(err)
			}
		}
		if err := session.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
