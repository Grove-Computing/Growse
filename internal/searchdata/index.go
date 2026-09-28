package searchdata

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/Grove-Computing/Growse/internal/omnibox"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type indexedEntry struct {
	source omnibox.Source
	text   string
	value  omnibox.Candidate
}

type localIndex struct{ entries []indexedEntry }

var indexFold = cases.Fold()

func normalizeText(value string) string {
	return norm.NFKC.String(indexFold.String(norm.NFKC.String(value)))
}

func rebuildIndex(data profileData) *localIndex {
	index := &localIndex{entries: make([]indexedEntry, 0, len(data.History)+len(data.Bookmarks))}
	for _, entry := range data.History {
		host := ""
		if parsed, err := url.Parse(entry.URL); err == nil {
			host = parsed.Hostname()
		}
		index.entries = append(index.entries, indexedEntry{
			source: omnibox.HistorySource,
			text:   normalizeText(entry.URL + "\x00" + host + "\x00" + entry.Title),
			value: omnibox.Candidate{
				Primary: entry.Title, URL: entry.URL, LastVisited: entry.LastVisited,
				VisitCount: entry.VisitCount, TypedCount: entry.TypedCount,
			},
		})
	}
	for _, entry := range data.Bookmarks {
		host := ""
		if parsed, err := url.Parse(entry.URL); err == nil {
			host = parsed.Hostname()
		}
		index.entries = append(index.entries, indexedEntry{
			source: omnibox.BookmarkSource,
			text:   normalizeText(entry.URL + "\x00" + host + "\x00" + entry.Title),
			value:  omnibox.Candidate{Primary: entry.Title, URL: entry.URL, LastVisited: entry.UpdatedAt},
		})
	}
	return index
}

// SuggestionSnapshot searches the derived index without retaining the query.
func (s *Store) SuggestionSnapshot(ctx context.Context, query string) omnibox.Snapshot {
	if s == nil || s.state == nil {
		return omnibox.Snapshot{}
	}
	s.state.mu.RLock()
	index := s.state.index
	if index == nil {
		index = &localIndex{}
	}
	entries := append([]indexedEntry(nil), index.entries...)
	s.state.mu.RUnlock()
	normalizedQuery := normalizeText(strings.TrimSpace(query))
	result := omnibox.Snapshot{}
	for offset, entry := range entries {
		if offset&63 == 0 {
			select {
			case <-ctx.Done():
				return omnibox.Snapshot{}
			default:
			}
		}
		if normalizedQuery != "" && !strings.Contains(entry.text, normalizedQuery) {
			continue
		}
		candidate := entry.value
		candidate.Source = entry.source
		if candidate.Primary == "" {
			candidate.Primary = candidate.URL
		}
		switch entry.source {
		case omnibox.HistorySource:
			result.History = append(result.History, candidate)
		case omnibox.BookmarkSource:
			result.Bookmarks = append(result.Bookmarks, candidate)
		}
	}
	sort.Slice(result.History, func(i, j int) bool {
		if !result.History[i].LastVisited.Equal(result.History[j].LastVisited) {
			return result.History[i].LastVisited.After(result.History[j].LastVisited)
		}
		return result.History[i].URL < result.History[j].URL
	})
	sort.Slice(result.Bookmarks, func(i, j int) bool {
		if !result.Bookmarks[i].LastVisited.Equal(result.Bookmarks[j].LastVisited) {
			return result.Bookmarks[i].LastVisited.After(result.Bookmarks[j].LastVisited)
		}
		return result.Bookmarks[i].URL < result.Bookmarks[j].URL
	})
	if len(result.History) > omnibox.MaxSourceCandidates {
		result.History = result.History[:omnibox.MaxSourceCandidates]
	}
	if len(result.Bookmarks) > omnibox.MaxSourceCandidates {
		result.Bookmarks = result.Bookmarks[:omnibox.MaxSourceCandidates]
	}
	return result
}

// LocalPipeline runs index searches outside the UI thread and rejects stale
// generations after query changes or lifecycle cancellation.
type LocalPipeline struct {
	mu         sync.Mutex
	store      *Store
	notify     func()
	generation uint64
	cancel     context.CancelFunc
	closed     bool
	result     omnibox.Snapshot
}

func NewLocalPipeline(store *Store, notify func()) *LocalPipeline {
	return &LocalPipeline{store: store, notify: notify}
}

func (p *LocalPipeline) Update(query string) uint64 {
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
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	store := p.store
	p.mu.Unlock()
	go func() {
		result := store.SuggestionSnapshot(ctx, query)
		p.mu.Lock()
		if p.closed || generation != p.generation || ctx.Err() != nil {
			p.mu.Unlock()
			return
		}
		p.result = result
		p.mu.Unlock()
		if p.notify != nil {
			p.notify()
		}
	}()
	return generation
}

func (p *LocalPipeline) Results() (uint64, omnibox.Snapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := p.result
	result.History = append([]omnibox.Candidate(nil), result.History...)
	result.Bookmarks = append([]omnibox.Candidate(nil), result.Bookmarks...)
	return p.generation, result
}

func (p *LocalPipeline) Cancel() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.generation++
	p.result = omnibox.Snapshot{}
}

func (p *LocalPipeline) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.closed = true
	p.generation++
	p.result = omnibox.Snapshot{}
}
