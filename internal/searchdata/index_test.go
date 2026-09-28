package searchdata

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/omnibox"
)

func TestIndexSearchesNormalizedURLHostAndTitleWithinBounds(t *testing.T) {
	store := NewMemoryStore()
	for index := range omnibox.MaxSourceCandidates + 20 {
		title := "unrelated"
		if index == 0 {
			title = "Ｍｅｔａ構文変数"
		}
		if err := store.RecordNavigation(Navigation{
			URL: fmt.Sprintf("https://docs.example.com/page/%d", index), Title: title,
			VisitedAt: time.Unix(int64(index+1), 0).UTC(), TopLevel: true, Success: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.SaveBookmark("", "https://日本語.example/guide", "案内"); err != nil {
		t.Fatal(err)
	}
	if got := store.SuggestionSnapshot(context.Background(), "meta構文"); len(got.History) != 1 || got.History[0].Primary != "Ｍｅｔａ構文変数" {
		t.Fatalf("normalized title results = %#v", got.History)
	}
	if got := store.SuggestionSnapshot(context.Background(), "xn--wgv71a119e.example"); len(got.Bookmarks) != 1 {
		t.Fatalf("normalized host results = %#v", got.Bookmarks)
	}
	if got := store.SuggestionSnapshot(context.Background(), ""); len(got.History) != omnibox.MaxSourceCandidates {
		t.Fatalf("bounded history results = %d", len(got.History))
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if got := store.SuggestionSnapshot(canceled, ""); len(got.History)+len(got.Bookmarks) != 0 {
		t.Fatalf("canceled results = %#v", got)
	}
}

func TestLocalPipelineRejectsStaleGeneration(t *testing.T) {
	notified := make(chan struct{}, 2)
	pipeline := NewLocalPipeline(nil, func() { notified <- struct{}{} })
	defer pipeline.Close()
	oldStarted := make(chan struct{})
	releaseOld := make(chan struct{})
	pipeline.search = func(ctx context.Context, query string) omnibox.Snapshot {
		if query == "old" {
			close(oldStarted)
			<-releaseOld
			return omnibox.Snapshot{History: []omnibox.Candidate{{Primary: "old"}}}
		}
		return omnibox.Snapshot{History: []omnibox.Candidate{{Primary: query}}}
	}
	pipeline.Update("old")
	<-oldStarted
	wantGeneration := pipeline.Update("new")
	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Fatal("live generation did not publish")
	}
	close(releaseOld)
	time.Sleep(10 * time.Millisecond)
	generation, result := pipeline.Results()
	if generation != wantGeneration || len(result.History) != 1 || result.History[0].Primary != "new" {
		t.Fatalf("pipeline result generation=%d result=%#v", generation, result)
	}
	pipeline.Cancel()
	if generation, result := pipeline.Results(); generation != 0 || len(result.History) != 0 {
		t.Fatalf("canceled pipeline generation=%d result=%#v", generation, result)
	}
}
