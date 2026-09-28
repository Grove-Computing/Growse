package searchdata

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const profileVersion = 1

var sharedProfiles sync.Map

type profileState struct {
	mu    sync.RWMutex
	data  profileData
	index *localIndex
}

// Store is a process-shared view of one browser profile.
type Store struct {
	path  string
	state *profileState
	now   func() time.Time
}

// NewMemoryStore creates a non-persistent store for tests or profile fallback.
func NewMemoryStore() *Store {
	data := profileData{Version: profileVersion}
	return &Store{state: &profileState{data: data, index: rebuildIndex(data)}, now: time.Now}
}

// OpenStore opens the profile's local search data file.
func OpenStore(root string) (*Store, error) {
	absolute, err := filepath.Abs(root)
	if err != nil || !filepath.IsAbs(absolute) {
		return nil, ErrStorage
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, ErrStorage
	}
	path := filepath.Join(absolute, "search-data.json")
	if current, ok := sharedProfiles.Load(path); ok {
		return &Store{path: path, state: current.(*profileState), now: time.Now}, nil
	}
	data, err := readProfile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		data = profileData{Version: profileVersion}
	}
	state := &profileState{data: data, index: rebuildIndex(data)}
	actual, _ := sharedProfiles.LoadOrStore(path, state)
	return &Store{path: path, state: actual.(*profileState), now: time.Now}, nil
}

func readProfile(path string) (profileData, error) {
	file, err := os.Open(path) // #nosec G304 -- fixed filename below the browser-owned profile root.
	if err != nil {
		return profileData{Version: profileVersion}, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, MaxProfileBytes+1))
	if err != nil || len(body) > MaxProfileBytes {
		return profileData{}, ErrStorage
	}
	var data profileData
	if json.Unmarshal(body, &data) != nil || validateProfile(data) != nil {
		return profileData{}, ErrInvalid
	}
	return data, nil
}

func validateProfile(data profileData) error {
	if data.Version != profileVersion || len(data.History) > MaxHistoryEntries || len(data.Bookmarks) > MaxBookmarkEntries {
		return ErrInvalid
	}
	seen := make(map[string]struct{}, len(data.History))
	for _, entry := range data.History {
		_, key, err := canonicalURL(entry.URL)
		if err != nil || !validTitle(entry.Title) || entry.LastVisited.IsZero() || entry.VisitCount == 0 {
			return ErrInvalid
		}
		if _, duplicate := seen[key]; duplicate {
			return ErrInvalid
		}
		seen[key] = struct{}{}
	}
	seen = make(map[string]struct{}, len(data.Bookmarks))
	for _, entry := range data.Bookmarks {
		_, key, err := canonicalURL(entry.URL)
		if err != nil || !validBookmark(entry) {
			return ErrInvalid
		}
		if _, duplicate := seen[key]; duplicate {
			return ErrInvalid
		}
		seen[key] = struct{}{}
	}
	return nil
}

func cloneProfile(data profileData) profileData {
	return profileData{
		Version:   profileVersion,
		History:   append([]HistoryEntry(nil), data.History...),
		Bookmarks: append([]Bookmark(nil), data.Bookmarks...),
	}
}

func writeProfile(path string, data profileData) error {
	body, err := json.Marshal(data)
	if err != nil || len(body) > MaxProfileBytes {
		return ErrLimit
	}
	if path == "" {
		return nil
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".search-data-*.tmp")
	if err != nil {
		return ErrStorage
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return ErrStorage
	}
	if _, err := temporary.Write(body); err != nil {
		return ErrStorage
	}
	if err := temporary.Sync(); err != nil {
		return ErrStorage
	}
	if err := temporary.Close(); err != nil {
		return ErrStorage
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return ErrStorage
	}
	committed = true
	if directory, err := os.Open(filepath.Dir(path)); err == nil { // #nosec G304 -- browser-owned profile directory.
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func (s *Store) update(mutate func(*profileData) error) error {
	if s == nil || s.state == nil {
		return ErrStorage
	}
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	next := cloneProfile(s.state.data)
	if err := mutate(&next); err != nil {
		return err
	}
	if err := validateProfile(next); err != nil {
		return err
	}
	if err := writeProfile(s.path, next); err != nil {
		return err
	}
	s.state.data = next
	s.state.index = rebuildIndex(next)
	return nil
}

// RecordNavigation adds or updates one successful top-level visit.
func (s *Store) RecordNavigation(navigation Navigation) error {
	if !navigation.Success || !navigation.TopLevel || navigation.Reload || navigation.SameDocument || navigation.RedirectIntermediate {
		return nil
	}
	stored, key, err := canonicalURL(navigation.URL)
	if err != nil || !validTitle(navigation.Title) {
		return ErrInvalid
	}
	visitedAt := navigation.VisitedAt
	if visitedAt.IsZero() {
		visitedAt = s.now().UTC()
	}
	return s.update(func(data *profileData) error {
		for index := range data.History {
			_, candidateKey, _ := canonicalURL(data.History[index].URL)
			if candidateKey != key {
				continue
			}
			entry := &data.History[index]
			entry.URL = stored
			if navigation.Title != "" {
				entry.Title = navigation.Title
			}
			entry.LastVisited = visitedAt
			entry.VisitCount = increment(entry.VisitCount)
			if navigation.Typed {
				entry.TypedCount = increment(entry.TypedCount)
			}
			return nil
		}
		if len(data.History) == MaxHistoryEntries {
			oldest := 0
			for index := 1; index < len(data.History); index++ {
				if data.History[index].LastVisited.Before(data.History[oldest].LastVisited) {
					oldest = index
				}
			}
			data.History = append(data.History[:oldest], data.History[oldest+1:]...)
		}
		entry := HistoryEntry{URL: stored, Title: navigation.Title, LastVisited: visitedAt, VisitCount: 1}
		if navigation.Typed {
			entry.TypedCount = 1
		}
		data.History = append(data.History, entry)
		return nil
	})
}

// History returns a newest-first immutable snapshot.
func (s *Store) History() []HistoryEntry {
	if s == nil || s.state == nil {
		return nil
	}
	s.state.mu.RLock()
	entries := append([]HistoryEntry(nil), s.state.data.History...)
	s.state.mu.RUnlock()
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].LastVisited.Equal(entries[j].LastVisited) {
			return entries[i].LastVisited.After(entries[j].LastVisited)
		}
		return entries[i].URL < entries[j].URL
	})
	return entries
}

// DeleteHistory removes one normalized URL from both profile data and index.
func (s *Store) DeleteHistory(rawURL string) error {
	_, key, err := canonicalURL(rawURL)
	if err != nil {
		return err
	}
	return s.update(func(data *profileData) error {
		kept := data.History[:0]
		for _, entry := range data.History {
			_, candidateKey, _ := canonicalURL(entry.URL)
			if candidateKey != key {
				kept = append(kept, entry)
			}
		}
		data.History = kept
		return nil
	})
}

// DeleteHistoryPeriod removes visits in the half-open [from, until) interval.
// A zero boundary leaves that side open.
func (s *Store) DeleteHistoryPeriod(from, until time.Time) error {
	if !from.IsZero() && !until.IsZero() && !from.Before(until) {
		return ErrInvalid
	}
	return s.update(func(data *profileData) error {
		kept := data.History[:0]
		for _, entry := range data.History {
			inside := (from.IsZero() || !entry.LastVisited.Before(from)) && (until.IsZero() || entry.LastVisited.Before(until))
			if !inside {
				kept = append(kept, entry)
			}
		}
		data.History = kept
		return nil
	})
}

// ClearHistory removes every history entry while preserving bookmarks.
func (s *Store) ClearHistory() error {
	return s.update(func(data *profileData) error {
		data.History = nil
		return nil
	})
}
