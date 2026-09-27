package omnibox

import (
	"strings"
	"testing"
)

func TestClassifyURLsAndSearchesDeterministically(t *testing.T) {
	for _, test := range []struct {
		input string
		kind  Kind
		url   string
	}{
		{" https://example.com:8443/a?q=b ", URL, "https://example.com:8443/a?q=b"},
		{"http://localhost:6053", URL, "http://localhost:6053"},
		{"127.0.0.1:8080/path", URL, "http://127.0.0.1:8080/path"},
		{"[::1]:8080/path", URL, "http://[::1]:8080/path"},
		{"example.co.jp/path", URL, "https://example.co.jp/path"},
		{"natural language query", Search, ""},
		{"日本語の検索", Search, ""},
		{"🦫 browser", Search, ""},
	} {
		got := Classify(test.input)
		if got.Kind != test.kind {
			t.Errorf("Classify(%q).Kind = %v, want %v (%+v)", test.input, got.Kind, test.kind, got)
			continue
		}
		if test.url != "" && got.URL.String() != test.url {
			t.Errorf("Classify(%q).URL = %q, want %q", test.input, got.URL, test.url)
		}
	}
}

func TestClassifyRejectsUnsafeOrAmbiguousURLShapedInput(t *testing.T) {
	tooLong := strings.Repeat("a", MaxInputBytes+1)
	for _, input := range []string{
		"https://user:secret@example.com", "ftp://example.com", "http://example.com:bad",
		"http://", "intranet:8080", "http://example.com\n", tooLong,
	} {
		got := Classify(input)
		if got.Kind != Invalid || got.Error == "" {
			t.Errorf("Classify(%q) = %+v, want visible invalid error", input, got)
		}
	}
}

func TestClassifyRecognizesOnlyLeadingReservedCommands(t *testing.T) {
	for _, test := range []struct {
		input string
		scope Scope
		query string
	}{
		{"@tabs", Tabs, ""},
		{"@history recent", History, "recent"},
		{"@bookmarks 日本語", Bookmarks, "日本語"},
		{"@search gopher", Web, "gopher"},
	} {
		got := Classify(test.input)
		if got.Kind != Command || got.Scope != test.scope || got.Query != test.query {
			t.Errorf("Classify(%q) = %+v", test.input, got)
		}
	}
	if got := Classify("search @tabs documentation"); got.Kind != Search {
		t.Errorf("embedded @ scope = %+v, want Search", got)
	}
}

func TestSearchURLEncodesQueryOnce(t *testing.T) {
	if got, want := SearchURL("日本語 + gopher").String(), "https://duckduckgo.com/?q=%E6%97%A5%E6%9C%AC%E8%AA%9E+%2B+gopher"; got != want {
		t.Fatalf("SearchURL = %q, want %q", got, want)
	}
}
