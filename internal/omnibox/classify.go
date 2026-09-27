// Package omnibox classifies address-bar input without sending ambiguous input
// to a search provider.
package omnibox

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxInputBytes bounds the text retained by the omnibox.
	MaxInputBytes = 4096
	// MaxQueryBytes bounds text included in a provider request.
	MaxQueryBytes = 2048
)

// Kind is the action determined from an omnibox submission.
type Kind uint8

const (
	Invalid Kind = iota
	URL
	Search
	Command
)

// Scope limits a command to one local source. An empty scope means no command.
type Scope string

const (
	Tabs      Scope = "tabs"
	History   Scope = "history"
	Bookmarks Scope = "bookmarks"
	Web       Scope = "search"
)

// Result is the lossless classification of one submission. Input is trimmed
// only at its edges; callers keep the editor's original text independently.
type Result struct {
	Kind  Kind
	Input string
	URL   *url.URL
	Query string
	Scope Scope
	Error string
}

// Classify deterministically separates URLs, web searches, local commands and
// invalid input. Invalid and ambiguous URL-shaped input is never a search.
func Classify(raw string) Result {
	if strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return invalid(Result{Input: strings.TrimSpace(raw)}, "制御文字を含む入力は実行できません")
	}
	input := strings.TrimSpace(raw)
	result := Result{Input: input}
	if input == "" {
		return invalid(result, "入力が空です")
	}
	if !utf8.ValidString(input) {
		return invalid(result, "入力は有効な UTF-8 ではありません")
	}
	if len(input) > MaxInputBytes {
		return invalid(result, fmt.Sprintf("入力は %d bytes を超えています", MaxInputBytes))
	}
	if scope, query, ok := command(input); ok {
		return Result{Kind: Command, Input: input, Query: query, Scope: scope}
	}
	if target, urlLike, err := urlTarget(input); urlLike {
		if err != nil {
			return invalid(result, err.Error())
		}
		result.Kind = URL
		result.URL = target
		return result
	}
	if len(input) > MaxQueryBytes {
		return invalid(result, fmt.Sprintf("検索語は %d bytes を超えています", MaxQueryBytes))
	}
	result.Kind = Search
	result.Query = input
	return result
}

// SearchURL produces the built-in DuckDuckGo request. Provider selection is
// intentionally outside this package so it can be replaced by profile state.
func SearchURL(query string) *url.URL {
	return &url.URL{Scheme: "https", Host: "duckduckgo.com", Path: "/", RawQuery: url.Values{"q": {query}}.Encode()}
}

func invalid(result Result, message string) Result {
	result.Kind = Invalid
	result.Error = message
	return result
}

func command(input string) (Scope, string, bool) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", "", false
	}
	var scope Scope
	switch parts[0] {
	case "@tabs":
		scope = Tabs
	case "@history":
		scope = History
	case "@bookmarks":
		scope = Bookmarks
	case "@search":
		scope = Web
	default:
		return "", "", false
	}
	return scope, strings.TrimSpace(strings.TrimPrefix(input, parts[0])), true
}

func urlTarget(input string) (*url.URL, bool, error) {
	if hasExplicitScheme(input) {
		target, err := url.ParseRequestURI(input)
		if err != nil {
			return nil, true, fmt.Errorf("URL が不正です: %w", err)
		}
		if target.Scheme != "http" && target.Scheme != "https" {
			return nil, true, fmt.Errorf("%s は許可されていない URL scheme です", target.Scheme)
		}
		if target.User != nil {
			return nil, true, fmt.Errorf("credential を含む URL は実行できません")
		}
		if target.Hostname() == "" {
			return nil, true, fmt.Errorf("URL host がありません")
		}
		return target, true, nil
	}

	// A valid scheme-looking prefix is never a natural-language search. This
	// includes unknown schemes such as mailto: and malformed http: URLs.
	if schemeLike(input) {
		return nil, true, fmt.Errorf("明示 URL は http または https を使用してください")
	}

	probe, err := url.Parse("http://" + input)
	if err != nil {
		if bareHostLike(input) {
			return nil, true, fmt.Errorf("URL が不正です: %w", err)
		}
		return nil, false, nil
	}
	host := probe.Hostname()
	if host == "" || probe.User != nil {
		return nil, false, nil
	}
	if host == "localhost" || net.ParseIP(host) != nil || validDottedHost(host) {
		if strings.Contains(input, "@") {
			return nil, true, fmt.Errorf("credential を含む URL は実行できません")
		}
		if host == "localhost" || net.ParseIP(host) != nil {
			probe.Scheme = "http"
		} else {
			probe.Scheme = "https"
		}
		return probe, true, nil
	}
	if bareHostLike(input) {
		return nil, true, fmt.Errorf("単一 label の host には http:// または https:// が必要です")
	}
	return nil, false, nil
}

func hasExplicitScheme(input string) bool {
	return strings.HasPrefix(strings.ToLower(input), "http://") || strings.HasPrefix(strings.ToLower(input), "https://")
}

func schemeLike(input string) bool {
	colon := strings.IndexByte(input, ':')
	if colon <= 0 {
		return false
	}
	for index, r := range input[:colon] {
		if !(unicode.IsLetter(r) || (index > 0 && (unicode.IsDigit(r) || r == '+' || r == '-' || r == '.'))) {
			return false
		}
	}
	return true
}

func validDottedHost(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-') {
				return false
			}
		}
	}
	return true
}

func bareHostLike(input string) bool {
	return !strings.ContainsAny(input, " /?#") && (strings.ContainsAny(input, ".:@") || strings.HasPrefix(input, "localhost"))
}
