package omnibox

import (
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
)

// MatchKind is ordered from weakest to strongest. Match quality dominates all
// source/frequency bonuses, so a substring cannot outrank an exact match.
type MatchKind uint8

const (
	NoMatch MatchKind = iota
	SubstringMatch
	WordBoundaryMatch
	PrefixMatch
	ExactMatch
)

// Match performs compatibility normalization and full Unicode case folding.
// CJK matches use substrings; whitespace tokenization is never required.
func Match(query, value string) MatchKind {
	query, value = normalized(query), normalized(value)
	if query == "" {
		return SubstringMatch
	}
	if query == value {
		return ExactMatch
	}
	if strings.HasPrefix(value, query) {
		return PrefixMatch
	}
	best := NoMatch
	for offset := 0; offset < len(value); {
		index := strings.Index(value[offset:], query)
		if index < 0 {
			break
		}
		index += offset
		best = SubstringMatch
		previous := []rune(value[:index])
		if len(previous) == 0 || !wordRune(previous[len(previous)-1]) {
			return WordBoundaryMatch
		}
		offset = index + len(query)
	}
	return best
}

func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_'
}

func candidateMatch(query string, c Candidate) MatchKind {
	best := max(Match(query, c.Primary), Match(query, c.URL))
	if u, err := url.Parse(c.URL); err == nil && u.Hostname() != "" {
		best = max(best, Match(query, u.Hostname()))
	}
	return best
}

func candidateScore(match MatchKind, c Candidate, now time.Time) int64 {
	weight := int64(100)
	switch c.Source {
	case InputSource:
		weight = 600
	case TabSource:
		weight = 500
	case BookmarkSource:
		weight = 400
	case HistorySource:
		weight = 200
		if c.TypedCount > 0 {
			weight = 300
		}
	}
	recency := int64(0)
	if !c.LastVisited.IsZero() {
		age := max(int64(now.Sub(c.LastVisited)/time.Hour), 0)
		recency = 1000 - min(age, 1000)
	}
	return int64(match)*1_000_000_000 + weight*1_000_000 + recency*1000 + int64(min(c.TypedCount, 500))*10 + int64(min(c.VisitCount, 500))
}

func candidateLess(a, b Candidate) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if candidateKey(a) != candidateKey(b) {
		return candidateKey(a) < candidateKey(b)
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	if a.Primary != b.Primary {
		return a.Primary < b.Primary
	}
	if a.URL != b.URL {
		return a.URL < b.URL
	}
	if a.TabID != b.TabID {
		return a.TabID < b.TabID
	}
	if !a.LastVisited.Equal(b.LastVisited) {
		return a.LastVisited.After(b.LastVisited)
	}
	if a.TypedCount != b.TypedCount {
		return a.TypedCount > b.TypedCount
	}
	return a.VisitCount > b.VisitCount
}

func rankCandidates(query string, candidates []Candidate, now time.Time) []Candidate {
	for i := range candidates {
		match := candidateMatch(query, candidates[i])
		if candidates[i].Source == InputSource {
			match = ExactMatch
		}
		if candidates[i].Source == RemoteSource {
			match = max(match, SubstringMatch)
		}
		candidates[i].Score = candidateScore(match, candidates[i], now)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidateLess(candidates[i], candidates[j]) })
	bounded := make([]Candidate, 0, min(len(candidates), MaxMergedCandidates))
	counts := make(map[Source]int)
	for _, c := range candidates {
		if counts[c.Source] >= MaxSourceCandidates {
			continue
		}
		counts[c.Source]++
		bounded = append(bounded, c)
	}
	bounded = deduplicate(bounded)
	sort.Slice(bounded, func(i, j int) bool { return candidateLess(bounded[i], bounded[j]) })
	if len(bounded) > MaxMergedCandidates {
		bounded = bounded[:MaxMergedCandidates]
	}
	return bounded
}
