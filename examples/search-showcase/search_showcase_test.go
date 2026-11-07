package main

import (
	"context"
	"io"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/findpage"
	"github.com/Grove-Computing/Growse/internal/homeconfig"
	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/Grove-Computing/Growse/internal/omnibox"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/yaegi"
	"github.com/Grove-Computing/Growse/internal/searchdata"
	"github.com/Grove-Computing/Growse/internal/searchprovider"
)

func TestSearchShowcaseReproducesEverySuggestionSourceOffline(t *testing.T) {
	showcase, err := newSearchShowcase(func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer showcase.Close()

	tabs := showcase.session.Tabs()
	if len(tabs) != 4 || !tabs[0].Active {
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

func TestSearchShowcaseFindFixtureCoversUnicodeNestedScrollAndMutation(t *testing.T) {
	server := newTLSServerForTest(t)
	engine := browser.NewWithRuntimeFactory(network.NewClientWithLimits(server.Client(), 1<<20), func() runtimemodel.Runtime { return yaegi.New() })
	defer engine.Close()
	page, err := engine.Navigate(t.Context(), server.URL+"/find")
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"interoperability", "検索対象", "Café", "nested needle"} {
		result := findpage.Search(page.Document, page.ComputedStyles, query, findpage.Options{}, findpage.Limits{})
		if len(result.Matches) == 0 || result.Limit != findpage.LimitNone {
			t.Errorf("Find(%q) = %+v", query, result)
		}
	}
	nested, ok := page.Document.GetElementByID("nested-target")
	if !ok || nested.TextContent() == "" {
		t.Fatal("nested scroll target is missing")
	}
	if before := findpage.Search(page.Document, page.ComputedStyles, "mutated needle", findpage.Options{}, findpage.Limits{}); len(before.Matches) != 0 {
		t.Fatalf("mutation text existed before click: %+v", before.Matches)
	}
	button, ok := page.Document.GetElementByID("mutate-find")
	if !ok {
		t.Fatal("mutation button is missing")
	}
	if !engine.DispatchClick(button.ID, 0, 0) {
		t.Fatalf("mutation click failed: started=%t error=%q scripts=%+v", page.RuntimeStarted, page.RuntimeError, page.ScriptErrors)
	}
	state, _ := page.Document.GetElementByID("mutation-state")
	after := findpage.Search(page.Document, page.ComputedStyles, "mutated needle", findpage.Options{}, findpage.Limits{})
	if state == nil || state.TextContent() != "mutation committed" || len(after.Matches) != 1 {
		t.Fatalf("mutation state=%v Find=%+v runtimeStarted=%t runtime=%q selected=%+v errors=%v", state, after, page.RuntimeStarted, page.RuntimeError, page.Scripts, page.ScriptErrors)
	}
}

func TestSearchShowcaseReproducesFailureDeletionAndProfileRecovery(t *testing.T) {
	showcase, err := newSearchShowcase(func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer showcase.Close()

	settings := showcase.chrome.SearchProviders()
	settings.RemoteSuggestions = false
	if err := showcase.chrome.SetSearchProviders(settings); err != nil || showcase.chrome.SearchProviders().RemoteSuggestions {
		t.Fatalf("provider opt-out = settings %+v error %v", showcase.chrome.SearchProviders(), err)
	}
	settings.RemoteSuggestions = true
	if err := showcase.chrome.SetSearchProviders(settings); err != nil || !showcase.chrome.SearchProviders().RemoteSuggestions {
		t.Fatalf("provider opt-in = settings %+v error %v", showcase.chrome.SearchProviders(), err)
	}

	timeoutContext, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	started := time.Now()
	if terms, err := showcase.remote(timeoutContext, "timeout"); err == nil || len(terms) != 0 || time.Since(started) > 3*time.Second {
		t.Fatalf("timeout response = terms %#v error %v elapsed %s", terms, err, time.Since(started))
	}
	if terms, err := showcase.remote(t.Context(), "oversize"); err != searchprovider.ErrResponse || len(terms) != 0 {
		t.Fatalf("oversize response = terms %#v error %v", terms, err)
	}

	if err := showcase.data.DeleteHistory(showcase.server.URL + "/history-guide"); err != nil {
		t.Fatal(err)
	}
	if err := showcase.data.DeleteBookmark(showcase.server.URL + "/bookmark-guide"); err != nil {
		t.Fatal(err)
	}
	local := showcase.data.SuggestionSnapshot(t.Context(), "guide")
	if len(local.History) != 0 || len(local.Bookmarks) != 0 {
		t.Fatalf("deleted local results remained indexed: %+v", local)
	}
	if err := showcase.data.RecordNavigation(searchdata.Navigation{URL: showcase.server.URL + "/history-guide", Title: "Guide history", VisitedAt: time.Now(), TopLevel: true, Success: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := showcase.data.SaveBookmark("", showcase.server.URL+"/bookmark-guide", "Guide bookmark"); err != nil {
		t.Fatal(err)
	}
	profile, err := os.ReadFile(showcase.profilePath("search-data.json"))
	if err != nil || strings.Contains(string(profile), `"index"`) {
		t.Fatalf("rebuildable search profile = error %v body %s", err, profile)
	}

	homeStore, err := homeconfig.OpenStore(showcase.profileRoot)
	if err != nil {
		t.Fatal(err)
	}
	first := homeconfig.Defaults()
	first.Background = homeconfig.BackgroundDusk
	if _, err := first.Add("Guide shortcut", showcase.server.URL+"/tab-guide"); err != nil {
		t.Fatal(err)
	}
	if err := homeStore.Save(first); err != nil {
		t.Fatal(err)
	}
	second := first.Clone()
	second.Background = homeconfig.BackgroundForest
	if err := homeStore.Save(second); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(showcase.profilePath("home-settings.json"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if recovered := homeStore.Load(); !reflect.DeepEqual(recovered, first) {
		t.Fatalf("home recovery = %+v, want %+v", recovered, first)
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
	for _, marker := range []string{"guide", "@tabs", "@history", "@bookmarks", "Shift", "Alt", "Home", "fake remote", "Public Internet", "timeout", "oversize", "History / Bookmark", "atomic"} {
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
