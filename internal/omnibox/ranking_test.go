package omnibox

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func TestUnicodeMatchKinds(t *testing.T) {
	for _, tc := range []struct {
		query, value string
		kind         MatchKind
	}{
		{"café", "CAFE\u0301", ExactMatch}, {"strasse", "Straße", ExactMatch},
		{"abc", "ＡＢＣ", ExactMatch}, {"Guide", "guide book", PrefixMatch},
		{"guide", "a guide book", WordBoundaryMatch}, {"guide", "myguidebook", SubstringMatch},
		{"日本語", "検索は日本語でできます", SubstringMatch}, {"🦊", "Fox 🦊", WordBoundaryMatch},
		{"absent", "other", NoMatch},
	} {
		if got := Match(tc.query, tc.value); got != tc.kind {
			t.Errorf("Match(%q,%q) = %v, want %v", tc.query, tc.value, got, tc.kind)
		}
	}
}

func TestRankingMatchDominatesSourceAndFrequency(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	snapshot := Snapshot{Now: now,
		Tabs:      []Candidate{{Primary: "myguidebook", URL: "https://substring.example/", TabID: 1, LastVisited: now, VisitCount: ^uint32(0)}},
		Bookmarks: []Candidate{{Primary: "a guide book", URL: "https://boundary.example/"}},
		History:   []Candidate{{Primary: "Guide", URL: "https://exact.example/"}, {Primary: "Guide book", URL: "https://prefix.example/"}},
	}
	got := Rank("guide", snapshot, nil)
	want := []string{"guide", "Guide", "Guide book", "a guide book", "myguidebook"}
	var titles []string
	for _, c := range got {
		titles = append(titles, c.Primary)
	}
	if !reflect.DeepEqual(titles, want) {
		t.Fatalf("ranking = %v, want %v", titles, want)
	}
}

func TestRankingSourceRecencyCountsAndStableTieBreak(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	snapshot := Snapshot{Now: now,
		Tabs:      []Candidate{{Primary: "guide", URL: "https://tab.example/", TabID: 1}},
		Bookmarks: []Candidate{{Primary: "guide", URL: "https://bookmark.example/"}},
		History: []Candidate{
			{Primary: "guide", URL: "https://typed.example/", TypedCount: 1},
			{Primary: "guide", URL: "https://older.example/", LastVisited: now.Add(-48 * time.Hour)},
			{Primary: "guide", URL: "https://recent.example/", LastVisited: now},
			{Primary: "guide", URL: "https://visited.example/", LastVisited: now, VisitCount: 20},
			{Primary: "guide", URL: "https://a-tie.example/"}, {Primary: "guide", URL: "https://z-tie.example/"},
		},
	}
	got := Rank("guide", snapshot, []string{"guide other"})
	want := []string{"", "https://tab.example/", "https://bookmark.example/", "https://typed.example/", "https://visited.example/", "https://recent.example/", "https://older.example/", "https://a-tie.example/", "https://z-tie.example/", ""}
	var urls []string
	for _, c := range got {
		urls = append(urls, c.URL)
	}
	if !reflect.DeepEqual(urls, want) {
		t.Fatalf("ranking = %v, want %v", urls, want)
	}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 20; i++ {
		rng.Shuffle(len(snapshot.History), func(i, j int) { snapshot.History[i], snapshot.History[j] = snapshot.History[j], snapshot.History[i] })
		if next := Rank("guide", snapshot, []string{"guide other"}); !reflect.DeepEqual(got, next) {
			t.Fatal("snapshot order changed scores or tie-break")
		}
	}
}

func TestRankingBoundsAfterMatchingAndScoring(t *testing.T) {
	snapshot := Snapshot{}
	for i := 0; i < 160; i++ {
		title := "a query"
		if i == 159 {
			title = "query"
		}
		c := Candidate{Primary: title, URL: fmt.Sprintf("https://example.com/%03d", i), TabID: uint64(i + 1)}
		snapshot.Tabs = append(snapshot.Tabs, c)
		snapshot.History = append(snapshot.History, Candidate{Primary: title, URL: fmt.Sprintf("https://history.example/%03d", i)})
		snapshot.Bookmarks = append(snapshot.Bookmarks, Candidate{Primary: title, URL: fmt.Sprintf("https://bookmark.example/%03d", i)})
	}
	got := Rank("query", snapshot, nil)
	if len(got) != MaxMergedCandidates {
		t.Fatalf("merged count = %d", len(got))
	}
	counts := map[Source]int{}
	found := false
	for _, c := range got {
		counts[c.Source]++
		if c.Source == TabSource && c.TabID == 160 {
			found = true
		}
	}
	for source, count := range counts {
		if count > MaxSourceCandidates {
			t.Fatalf("source %v count = %d", source, count)
		}
	}
	if !found {
		t.Fatal("source truncation hid its best match")
	}
}
