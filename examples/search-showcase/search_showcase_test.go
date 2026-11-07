package main

import (
	"context"
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/omnibox"
)

func TestSearchShowcaseReproducesEverySuggestionSourceOffline(t *testing.T) {
	showcase, err := newSearchShowcase(func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer showcase.Close()

	tabs := showcase.session.Tabs()
	if len(tabs) != 3 || !tabs[0].Active {
		t.Fatalf("fixture tabs = %+v", tabs)
	}
	snapshot := showcase.data.SuggestionSnapshot(t.Context(), "guide")
	for _, tab := range tabs {
		if tab.URL != "" {
			snapshot.Tabs = append(snapshot.Tabs, omnibox.Candidate{Primary: tab.Title, URL: tab.URL, TabID: uint64(tab.ID)})
		}
	}
	remote, err := showcase.remote(context.Background(), "guide")
	if err != nil {
		t.Fatal(err)
	}
	ranked := omnibox.Rank("guide", snapshot, remote)
	got := make(map[omnibox.Source]bool)
	for _, candidate := range ranked {
		got[candidate.Source] = true
	}
	for _, source := range []omnibox.Source{omnibox.InputSource, omnibox.TabSource, omnibox.HistorySource, omnibox.BookmarkSource, omnibox.RemoteSource} {
		if !got[source] {
			t.Fatalf("source %s missing from ranked candidates: %+v", source.Label(), ranked)
		}
	}
	if second := omnibox.Rank("guide", snapshot, remote); !reflect.DeepEqual(ranked, second) {
		t.Fatalf("ranking changed between identical runs:\nfirst=%+v\nsecond=%+v", ranked, second)
	}
	provider := showcase.settings.Default()
	target, err := provider.SearchURL("gopher browser")
	if err != nil || !strings.HasPrefix(target, showcase.server.URL+"/search?") {
		t.Fatalf("offline search target = %q, %v", target, err)
	}
}

func TestSearchShowcaseDocumentsClassificationHomeAndDispositionSteps(t *testing.T) {
	server := newTLSServerForTest(t)
	response, err := server.Client().Get(server.URL + "/tab-guide")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, marker := range []string{"guide", "@tabs", "@history", "@bookmarks", "Shift", "Alt", "Home", "fake remote", "Public Internet"} {
		if !strings.Contains(text, marker) {
			t.Errorf("showcase instructions missing %q", marker)
		}
	}
	for _, test := range []struct {
		input string
		kind  omnibox.Kind
	}{{"guide", omnibox.Search}, {"@tabs guide", omnibox.Command}, {server.URL + "/tab-guide", omnibox.URL}} {
		if got := omnibox.Classify(test.input).Kind; got != test.kind {
			t.Errorf("Classify(%q) = %v, want %v", test.input, got, test.kind)
		}
	}
}

func newTLSServerForTest(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(searchShowcaseHandler())
	t.Cleanup(server.Close)
	return server
}
