package omnibox

import (
	"reflect"
	"testing"
)

func TestNormalizeURLIdentity(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"https://EXAMPLE.com:443/#section", "https://example.com"},
		{"http://EXAMPLE.com:80/a/", "http://example.com/a"},
		{"https://例え.テスト/", "https://xn--r8jz45g.xn--zckzah"},
		{"http://[0:0:0:0:0:0:0:1]:80/", "http://[::1]"},
		{"https://example.com/A?q=a%2Fb#one", "https://example.com/A?q=a%2Fb"},
		{"https://example.com/a%2Fb/", "https://example.com/a%2Fb"},
		{"https://example.com/?", "https://example.com?"},
	} {
		got, err := NormalizeURL(tc.input)
		if err != nil || got != tc.want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{"javascript:bad", "https://user:pass@example.com/", "https://example.com:99999", "https://example.com:bad", "/relative"} {
		if _, err := NormalizeURL(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestDeduplicationAcrossSourcesPreservesDistinctTargets(t *testing.T) {
	candidates := []Candidate{
		{Source: HistorySource, URL: "https://EXAMPLE.com:443/a/#one"},
		{Source: BookmarkSource, URL: "https://example.com/a#two"},
		{Source: TabSource, URL: "https://example.com/a/", TabID: 7},
		{Source: HistorySource, URL: "http://example.com/a"},
		{Source: HistorySource, URL: "https://example.com:444/a"},
		{Source: HistorySource, URL: "https://example.com/A"},
		{Source: HistorySource, URL: "https://other.example/a"},
		{Source: HistorySource, URL: "https://example.com/a?q=1"},
		{Source: HistorySource, URL: "https://example.com/a?q=2"},
		{Source: HistorySource, URL: "https://example.com/a%2Fb"},
		{Source: HistorySource, URL: "https://example.com/a/b"},
		{Source: InputSource, Query: "CAFÉ"},
		{Source: RemoteSource, Query: "cafe\u0301"},
	}
	result := deduplicate(candidates)
	if len(result) != 10 || result[0].Source != TabSource || result[0].TabID != 7 {
		t.Fatalf("deduplicated = %+v", result)
	}
	reversed := append([]Candidate(nil), candidates...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	a, b := map[string]Candidate{}, map[string]Candidate{}
	for _, c := range result {
		a[candidateKey(c)] = c
	}
	for _, c := range deduplicate(reversed) {
		b[candidateKey(c)] = c
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("deduplication depends on source iteration order")
	}
}
