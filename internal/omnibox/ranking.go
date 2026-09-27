package omnibox

import (
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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
	return matchNormalized(normalized(query), normalized(value))
}

func matchNormalized(query, value string) MatchKind {
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
		previous, _ := utf8.DecodeLastRuneInString(value[:index])
		if index == 0 || !wordRune(previous) {
			return WordBoundaryMatch
		}
		offset = index + len(query)
	}
	return best
}

func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_'
}

func candidateMatchNormalized(query string, c Candidate) MatchKind {
	best := max(matchNormalized(query, normalized(c.Primary)), matchNormalized(query, normalized(c.URL)))
	if best == ExactMatch {
		return best
	}
	if u, err := url.Parse(c.URL); err == nil && u.Hostname() != "" {
		best = max(best, matchNormalized(query, normalized(u.Hostname())))
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
	return candidateTieLess(a, b, candidateKey(a), candidateKey(b))
}

func candidateTieLess(a, b Candidate, aKey, bKey string) bool {
	if aKey != bKey {
		return aKey < bKey
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

type rankedCandidate struct {
	candidate Candidate
	key       string
}

func rankedLess(a, b rankedCandidate) bool {
	if a.candidate.Score != b.candidate.Score {
		return a.candidate.Score > b.candidate.Score
	}
	return candidateTieLess(a.candidate, b.candidate, a.key, b.key)
}

type rankingAccumulator map[Source][]rankedCandidate

func (r rankingAccumulator) add(c Candidate) {
	// Keep only the best 50 per source. URL identities are cached for admitted
	// entries; candidates below the score floor need no normalization or sort.
	group := r[c.Source]
	if len(group) == MaxSourceCandidates && c.Score < group[len(group)-1].candidate.Score {
		return
	}
	ranked := rankedCandidate{candidate: c, key: candidateKey(c)}
	index := sort.Search(len(group), func(i int) bool { return rankedLess(ranked, group[i]) })
	if index >= MaxSourceCandidates {
		return
	}
	group = append(group, rankedCandidate{})
	copy(group[index+1:], group[index:])
	group[index] = ranked
	if len(group) > MaxSourceCandidates {
		group = group[:MaxSourceCandidates]
	}
	r[c.Source] = group
}

func (r rankingAccumulator) results() []Candidate {
	bounded := make([]Candidate, 0, MaxMergedCandidates)
	for _, group := range r {
		for _, ranked := range group {
			bounded = append(bounded, ranked.candidate)
		}
	}
	bounded = deduplicate(bounded)
	sort.Slice(bounded, func(i, j int) bool { return candidateLess(bounded[i], bounded[j]) })
	if len(bounded) > MaxMergedCandidates {
		bounded = bounded[:MaxMergedCandidates]
	}
	return bounded
}
