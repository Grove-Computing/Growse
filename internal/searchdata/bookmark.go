package searchdata

import "sort"

// SaveBookmark adds or updates one flat bookmark. originalURL is empty when
// adding; editing to an existing normalized URL merges both records.
func (s *Store) SaveBookmark(originalURL, rawURL, title string) (Bookmark, error) {
	stored, key, err := canonicalURL(rawURL)
	if err != nil || !validTitle(title) {
		return Bookmark{}, ErrInvalid
	}
	originalKey := ""
	if originalURL != "" {
		if _, originalKey, err = canonicalURL(originalURL); err != nil {
			return Bookmark{}, ErrInvalid
		}
	}
	now := s.now().UTC()
	var saved Bookmark
	err = s.update(func(data *profileData) error {
		createdAt := now
		kept := data.Bookmarks[:0]
		for _, entry := range data.Bookmarks {
			_, candidateKey, _ := canonicalURL(entry.URL)
			if candidateKey == originalKey || candidateKey == key {
				if entry.CreatedAt.Before(createdAt) {
					createdAt = entry.CreatedAt
				}
				continue
			}
			kept = append(kept, entry)
		}
		data.Bookmarks = kept
		if len(data.Bookmarks) >= MaxBookmarkEntries {
			return ErrLimit
		}
		saved = Bookmark{URL: stored, Title: title, CreatedAt: createdAt, UpdatedAt: now}
		data.Bookmarks = append(data.Bookmarks, saved)
		return nil
	})
	return saved, err
}

// DeleteBookmark removes one normalized URL from profile data and index.
func (s *Store) DeleteBookmark(rawURL string) error {
	_, key, err := canonicalURL(rawURL)
	if err != nil {
		return err
	}
	return s.update(func(data *profileData) error {
		kept := data.Bookmarks[:0]
		for _, entry := range data.Bookmarks {
			_, candidateKey, _ := canonicalURL(entry.URL)
			if candidateKey != key {
				kept = append(kept, entry)
			}
		}
		data.Bookmarks = kept
		return nil
	})
}

// Bookmark returns one item by normalized URL.
func (s *Store) Bookmark(rawURL string) (Bookmark, bool) {
	_, key, err := canonicalURL(rawURL)
	if err != nil || s == nil || s.state == nil {
		return Bookmark{}, false
	}
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	for _, entry := range s.state.data.Bookmarks {
		_, candidateKey, _ := canonicalURL(entry.URL)
		if candidateKey == key {
			return entry, true
		}
	}
	return Bookmark{}, false
}

// Bookmarks returns an updated-first immutable snapshot.
func (s *Store) Bookmarks() []Bookmark {
	if s == nil || s.state == nil {
		return nil
	}
	s.state.mu.RLock()
	entries := append([]Bookmark(nil), s.state.data.Bookmarks...)
	s.state.mu.RUnlock()
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].UpdatedAt.Equal(entries[j].UpdatedAt) {
			return entries[i].UpdatedAt.After(entries[j].UpdatedAt)
		}
		return entries[i].URL < entries[j].URL
	})
	return entries
}

func validBookmark(entry Bookmark) bool {
	_, _, err := canonicalURL(entry.URL)
	return err == nil && validTitle(entry.Title) && !entry.CreatedAt.IsZero() && !entry.UpdatedAt.IsZero() && !entry.UpdatedAt.Before(entry.CreatedAt)
}
