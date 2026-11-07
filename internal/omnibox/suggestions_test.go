package omnibox

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestPipelineLocalSourcesAndScopes(t *testing.T) {
	snapshot := Snapshot{
		Tabs:      []Candidate{{Primary: "日本語 Guide", URL: "https://tab.example/", TabID: 7}},
		History:   []Candidate{{Primary: "日本語 history", URL: "https://history.example/"}},
		Bookmarks: []Candidate{{Primary: "日本語 bookmark", URL: "https://bookmark.example/"}},
	}
	for _, tc := range []struct {
		input   string
		sources []Source
	}{
		{"日本語", []Source{InputSource, TabSource, BookmarkSource, HistorySource}},
		{"@tabs 日本", []Source{TabSource}}, {"@history", []Source{HistorySource}},
		{"@bookmarks 日本", []Source{BookmarkSource}}, {"@search 日本", []Source{InputSource}},
		{"https://tab.example/", []Source{TabSource}}, {"javascript:bad", nil},
	} {
		t.Run(tc.input, func(t *testing.T) {
			p := NewPipeline(nil)
			defer p.Close()
			p.Update(tc.input, snapshot, nil, false)
			_, candidates := p.Results()
			var sources []Source
			for _, c := range candidates {
				sources = append(sources, c.Source)
			}
			if !reflect.DeepEqual(sources, tc.sources) {
				t.Fatalf("sources = %v, want %v", sources, tc.sources)
			}
		})
	}
}

func TestPipelineRemoteLifecycle(t *testing.T) {
	for _, lifecycle := range []string{"input", "cancel", "close"} {
		t.Run(lifecycle, func(t *testing.T) {
			p := NewPipeline(nil)
			defer p.Close()
			started, release, done := make(chan context.Context, 1), make(chan struct{}), make(chan struct{})
			p.Update("old query", Snapshot{}, func(ctx context.Context, _ string) ([]string, error) {
				started <- ctx
				<-release
				close(done)
				return []string{"stale query"}, nil
			}, true)
			var ctx context.Context
			select {
			case ctx = <-started:
			case <-time.After(time.Second):
				t.Fatal("request did not start")
			}
			switch lifecycle {
			case "input":
				p.Update("new query", Snapshot{}, nil, false)
			case "cancel":
				p.Cancel()
			case "close":
				p.Close()
			}
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("not canceled")
			}
			close(release)
			<-done
			_, results := p.Results()
			for _, c := range results {
				if c.Query == "stale query" {
					t.Fatal("stale response published")
				}
			}
			if lifecycle == "input" && (len(results) != 1 || results[0].Query != "new query") {
				t.Fatalf("local input lost: %v", results)
			}
		})
	}
}

func TestPipelineRemoteSuccessFailureAndPrivacy(t *testing.T) {
	for _, fail := range []bool{false, true} {
		p := NewPipeline(nil)
		done := make(chan struct{})
		p.Update("local query", Snapshot{}, func(ctx context.Context, query string) ([]string, error) {
			if query != "local query" {
				t.Error("wrong query")
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Error("no request timeout")
			}
			defer close(done)
			if fail {
				return nil, errors.New("unavailable")
			}
			return []string{"remote query", "javascript:bad", "https://url.example/"}, nil
		}, true)
		<-done
		// Synchronize with publication without assuming scheduler timing.
		deadline := time.Now().Add(time.Second)
		for {
			_, results := p.Results()
			if fail {
				if len(results) != 1 {
					t.Fatal("failure removed local result")
				}
				break
			}
			if len(results) == 2 {
				if results[1].Query != "remote query" {
					t.Fatal(results)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("remote result missing")
			}
			time.Sleep(time.Millisecond)
		}
		p.Close()
	}
	for _, tc := range []struct {
		input   string
		enabled bool
	}{
		{"", true}, {"https://example.com/", true}, {"localhost", true}, {"127.0.0.1", true},
		{"@search query", true}, {"user:password@host", true}, {"bad\nquery", true}, {"private query", false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			p := NewPipeline(nil)
			defer p.Close()
			called := make(chan struct{}, 1)
			fetch := func(context.Context, string) ([]string, error) { called <- struct{}{}; return nil, nil }
			p.Update(tc.input, Snapshot{}, fetch, tc.enabled)
			select {
			case <-called:
				t.Fatal("private input sent")
			case <-time.After(SuggestionDebounce + 20*time.Millisecond):
			}
		})
	}
}

func TestPipelineDebounceAndTimeoutKeepLocalResults(t *testing.T) {
	p := NewPipeline(nil)
	defer p.Close()
	started := make(chan string, 1)
	done := make(chan struct{})
	fetch := func(ctx context.Context, query string) ([]string, error) {
		started <- query
		<-ctx.Done()
		close(done)
		return []string{"late result"}, nil
	}
	p.Update("old query", Snapshot{}, fetch, true)
	p.Update("new query", Snapshot{}, fetch, true)
	select {
	case query := <-started:
		if query != "new query" {
			t.Fatal("debounce fetched obsolete query")
		}
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	select {
	case <-done:
	case <-time.After(SuggestionTimeout + time.Second):
		t.Fatal("timeout did not cancel")
	}
	_, results := p.Results()
	if len(results) != 1 || results[0].Query != "new query" {
		t.Fatal("timeout changed local input action")
	}
}
