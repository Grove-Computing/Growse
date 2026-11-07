// Package searchdata owns profile-scoped browsing history, bookmarks, and the
// rebuildable local search index. It never stores page bodies or search queries.
package searchdata

import (
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Grove-Computing/Growse/internal/omnibox"
)

const (
	MaxHistoryEntries  = 50_000
	MaxBookmarkEntries = 10_000
	MaxProfileBytes    = 64 * 1024 * 1024
	MaxURLBytes        = 4 * 1024
	MaxTitleBytes      = 16 * 1024
)

var (
	ErrInvalid = errors.New("invalid local search data")
	ErrLimit   = errors.New("local search data limit exceeded")
	ErrStorage = errors.New("local search data storage failed")
)

// HistoryEntry is one deduplicated, successfully committed top-level visit.
type HistoryEntry struct {
	URL         string    `json:"url"`
	Title       string    `json:"title,omitempty"`
	LastVisited time.Time `json:"lastVisited"`
	VisitCount  uint32    `json:"visitCount"`
	TypedCount  uint32    `json:"typedCount,omitempty"`
}

// Bookmark is one item in the profile's flat bookmark collection.
type Bookmark struct {
	URL       string    `json:"url"`
	Title     string    `json:"title,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Navigation describes a completed browser navigation. Non-qualifying events
// are accepted as no-ops so failure, reload, fragment, and redirect paths never
// create profile entries accidentally.
type Navigation struct {
	URL                  string
	Title                string
	VisitedAt            time.Time
	Typed                bool
	TopLevel             bool
	Success              bool
	Reload               bool
	SameDocument         bool
	RedirectIntermediate bool
}

type profileData struct {
	Version   int            `json:"version"`
	History   []HistoryEntry `json:"history,omitempty"`
	Bookmarks []Bookmark     `json:"bookmarks,omitempty"`
}

func canonicalURL(raw string) (string, string, error) {
	if len(raw) == 0 || len(raw) > MaxURLBytes || !utf8.ValidString(raw) {
		return "", "", ErrInvalid
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || parsed.Hostname() == "" {
		return "", "", ErrInvalid
	}
	if scheme := strings.ToLower(parsed.Scheme); scheme != "http" && scheme != "https" {
		return "", "", ErrInvalid
	}
	parsed.Fragment = ""
	parsed.RawFragment = ""
	stored := parsed.String()
	key, err := omnibox.NormalizeURL(stored)
	if err != nil {
		return "", "", ErrInvalid
	}
	return stored, key, nil
}

func validTitle(title string) bool {
	return utf8.ValidString(title) && len(title) <= MaxTitleBytes
}

func increment(value uint32) uint32 {
	if value != ^uint32(0) {
		value++
	}
	return value
}
