package omnibox

import (
	"context"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	MaxSourceCandidates  = 50
	MaxMergedCandidates  = 100
	MaxVisibleCandidates = 12
	SuggestionDebounce   = 150 * time.Millisecond
	SuggestionTimeout    = 2 * time.Second
)

// Source identifies both the candidate's label and its execution semantics.
type Source uint8

const (
	InputSource Source = iota
	TabSource
	BookmarkSource
	HistorySource
	RemoteSource
)

func (s Source) Label() string {
	switch s {
	case TabSource:
		return "Tab"
	case BookmarkSource:
		return "Bookmark"
	case HistorySource:
		return "History"
	case RemoteSource:
		return "Suggestion"
	default:
		return "Input"
	}
}

// Candidate is a value snapshot. URL actions navigate; Tab actions select TabID;
// Query actions search with the current provider. No response markup is rendered.
type Candidate struct {
	Source                 Source
	Primary, Secondary     string
	URL, Query             string
	TabID                  uint64
	LastVisited            time.Time
	VisitCount, TypedCount uint32
	Score                  int64
}

// Snapshot is shared by the omnibox and future search surfaces. Persistence is
// owned by the source, not by the pipeline; no query or page text is retained.
type Snapshot struct{ Tabs, History, Bookmarks []Candidate }

// RemoteFetcher is provided by the provider layer after endpoint validation.
// It must honor cancellation and enforce transport/response bounds.
type RemoteFetcher func(context.Context, string) ([]string, error)

func normalized(value string) string {
	return cases.Fold().String(norm.NFKC.String(value))
}

func localCandidates(input string, snapshot Snapshot, remote []string) []Candidate {
	classification := Classify(input)
	if classification.Kind == Invalid && strings.TrimSpace(input) != "" {
		return nil
	}
	query, scope := classification.Query, classification.Scope
	if classification.Kind == URL {
		query = classification.Input
	}
	result := make([]Candidate, 0, MaxMergedCandidates)
	if classification.Kind == URL {
		result = append(result, Candidate{Source: InputSource, Primary: classification.Input, Secondary: "Open URL", URL: classification.URL.String()})
	} else if query != "" && (scope == "" || scope == Web) {
		result = append(result, Candidate{Source: InputSource, Primary: query, Secondary: "Search", Query: query})
	}
	sources := []struct {
		kind    Source
		scope   Scope
		entries []Candidate
	}{
		{TabSource, Tabs, snapshot.Tabs}, {BookmarkSource, Bookmarks, snapshot.Bookmarks}, {HistorySource, History, snapshot.History},
	}
	needle := normalized(query)
	for _, source := range sources {
		if scope != "" && scope != source.scope {
			continue
		}
		count := 0
		for _, entry := range source.entries {
			if len(entry.URL) > MaxInputBytes || len(entry.Primary) > MaxInputBytes || (Classify(entry.URL).Kind != URL && !(source.kind == TabSource && entry.URL == "" && entry.TabID != 0)) {
				continue
			}
			if needle != "" && !strings.Contains(normalized(entry.Primary), needle) && !strings.Contains(normalized(entry.URL), needle) {
				continue
			}
			entry.Source = source.kind
			entry.Secondary = entry.URL
			if entry.Primary == "" {
				entry.Primary = entry.URL
			}
			result = append(result, entry)
			count++
			if count == MaxSourceCandidates {
				break
			}
		}
	}
	if scope == "" || scope == Web {
		count := 0
		for _, term := range remote {
			if len(term) > MaxQueryBytes || Classify(term).Kind != Search {
				continue
			}
			result = append(result, Candidate{Source: RemoteSource, Primary: term, Secondary: "Search suggestion", Query: term})
			count++
			if count == MaxSourceCandidates {
				break
			}
		}
	}
	if len(result) > MaxMergedCandidates {
		result = result[:MaxMergedCandidates]
	}
	return result
}

// Pipeline publishes local results synchronously and remote results only for the
// live generation. Its callback requests a UI frame; it never mutates UI state.
type Pipeline struct {
	mu         sync.Mutex
	generation uint64
	cancel     context.CancelFunc
	closed     bool
	candidates []Candidate
	notify     func()
}

func NewPipeline(notify func()) *Pipeline { return &Pipeline{notify: notify} }

func (p *Pipeline) Update(input string, snapshot Snapshot, fetch RemoteFetcher, enabled bool) uint64 {
	p.mu.Lock()
	if p.closed {
		generation := p.generation
		p.mu.Unlock()
		return generation
	}
	if p.cancel != nil {
		p.cancel()
	}
	p.generation++
	generation := p.generation
	p.candidates = localCandidates(input, snapshot, nil)
	// Own the snapshot used by a later remote completion.
	snapshot = Snapshot{append([]Candidate(nil), snapshot.Tabs...), append([]Candidate(nil), snapshot.History...), append([]Candidate(nil), snapshot.Bookmarks...)}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.mu.Unlock()
	classification := Classify(input)
	if !enabled || fetch == nil || classification.Kind != Search {
		return generation
	}
	go func() {
		timer := time.NewTimer(SuggestionDebounce)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		requestContext, requestCancel := context.WithTimeout(ctx, SuggestionTimeout)
		defer requestCancel()
		terms, err := fetch(requestContext, classification.Query)
		if err != nil || requestContext.Err() != nil {
			return
		}
		if len(terms) > MaxSourceCandidates {
			terms = terms[:MaxSourceCandidates]
		}
		candidates := localCandidates(input, snapshot, terms)
		p.mu.Lock()
		if p.closed || generation != p.generation || ctx.Err() != nil {
			p.mu.Unlock()
			return
		}
		p.candidates = candidates
		p.mu.Unlock()
		if p.notify != nil {
			p.notify()
		}
	}()
	return generation
}

func (p *Pipeline) Results() (uint64, []Candidate) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.generation, append([]Candidate(nil), p.candidates...)
}

// Cancel invalidates responses on popup close, tab switch or provider change.
func (p *Pipeline) Cancel() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.generation++
	p.candidates = nil
}

func (p *Pipeline) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.closed = true
	p.generation++
	p.candidates = nil
}
