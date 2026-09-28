// Package findpage searches visible DOM text without modifying the document.
package findpage

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

const (
	// DefaultMaxMatches bounds retained match metadata for one tab.
	DefaultMaxMatches = 10_000
	// DefaultMaxTextBytes bounds visible source text inspected by one search.
	DefaultMaxTextBytes = 16 << 20
)

// Options selects optional matching behavior.
type Options struct {
	CaseSensitive bool
	WholeWord     bool
}

// Limits bounds the amount of document data retained and inspected.
type Limits struct {
	MaxMatches   int
	MaxTextBytes int
}

// Match identifies one source range in a DOM text node. Start and End are UTF-8
// byte offsets and always fall on normalization-segment boundaries.
type Match struct {
	NodeID dom.NodeID
	Start  int
	End    int
}

// LimitKind identifies the safety bound that stopped a search.
type LimitKind uint8

const (
	LimitNone LimitKind = iota
	LimitMatches
	LimitTextBytes
)

// Result is an immutable search snapshot in DOM order.
type Result struct {
	Matches         []Match
	SearchableBytes int
	Limit           LimitKind
}

// Search visits visible text nodes in DOM order and returns normalized
// substring matches. It excludes browser controls and content that is not
// represented by ordinary visible DOM text.
func Search(document *dom.Document, styles stylemodel.Map, query string, options Options, limits Limits) Result {
	limits = normalizedLimits(limits)
	needle := normalize(query, options.CaseSensitive)
	if needle.text == "" || document == nil || document.Root == nil {
		return Result{}
	}

	result := Result{Matches: make([]Match, 0)}
	var visit func(*dom.Node, bool)
	visit = func(node *dom.Node, excluded bool) {
		if node == nil || result.Limit != LimitNone {
			return
		}
		if node.Type == dom.NodeElement {
			excluded = excluded || excludesText(node, styles)
		}
		if node.Type == dom.NodeText {
			if excluded || node.Text == "" {
				return
			}
			remaining := limits.MaxTextBytes - result.SearchableBytes
			if remaining <= 0 || len(node.Text) > remaining {
				result.Limit = LimitTextBytes
				return
			}
			result.SearchableBytes += len(node.Text)
			haystack := normalize(node.Text, options.CaseSensitive)
			for offset := 0; offset <= len(haystack.text)-len(needle.text); {
				relative := strings.Index(haystack.text[offset:], needle.text)
				if relative < 0 {
					break
				}
				start := offset + relative
				end := start + len(needle.text)
				startSource, startsAtBoundary := haystack.boundaries[start]
				endSource, endsAtBoundary := haystack.boundaries[end]
				if startsAtBoundary && endsAtBoundary && (!options.WholeWord || wholeWord(haystack.text, start, end)) {
					result.Matches = append(result.Matches, Match{NodeID: node.ID, Start: startSource, End: endSource})
					if len(result.Matches) >= limits.MaxMatches {
						result.Limit = LimitMatches
						return
					}
				}
				offset = start + 1
			}
			return
		}
		for _, child := range node.Children {
			visit(child, excluded)
			if result.Limit != LimitNone {
				return
			}
		}
	}
	visit(document.Root, false)
	return result
}

func normalizedLimits(limits Limits) Limits {
	if limits.MaxMatches <= 0 {
		limits.MaxMatches = DefaultMaxMatches
	}
	if limits.MaxTextBytes <= 0 {
		limits.MaxTextBytes = DefaultMaxTextBytes
	}
	return limits
}

var excludedElements = map[string]bool{
	"script": true, "style": true, "template": true, "noscript": true,
	"input": true, "textarea": true, "select": true, "option": true,
	"canvas": true,
}

func excludesText(node *dom.Node, styles stylemodel.Map) bool {
	if excludedElements[strings.ToLower(node.TagName)] {
		return true
	}
	if _, hidden := node.Attribute("hidden"); hidden {
		return true
	}
	computed, ok := styles.For(node)
	return ok && (computed.Display == stylemodel.DisplayNone || computed.Visibility == stylemodel.VisibilityHidden)
}

type normalizedText struct {
	text       string
	boundaries map[int]int
}

func normalize(source string, caseSensitive bool) normalizedText {
	var iterator norm.Iter
	iterator.InitString(norm.NFC, source)
	var builder strings.Builder
	boundaries := map[int]int{0: 0}
	for !iterator.Done() {
		sourceStart := iterator.Pos()
		segment := string(iterator.Next())
		sourceEnd := iterator.Pos()
		if !caseSensitive {
			segment = cases.Fold().String(segment)
		}
		boundaries[builder.Len()] = sourceStart
		builder.WriteString(segment)
		boundaries[builder.Len()] = sourceEnd
	}
	return normalizedText{text: builder.String(), boundaries: boundaries}
}

func wholeWord(text string, start, end int) bool {
	if start > 0 {
		previous, _ := utf8.DecodeLastRuneInString(text[:start])
		if isWordRune(previous) {
			return false
		}
	}
	if end < len(text) {
		next, _ := utf8.DecodeRuneInString(text[end:])
		if isWordRune(next) {
			return false
		}
	}
	return true
}

func isWordRune(value rune) bool {
	return value == '_' || unicode.IsLetter(value) || unicode.IsNumber(value) || unicode.IsMark(value)
}
