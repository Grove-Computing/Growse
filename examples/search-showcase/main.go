package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/Grove-Computing/Growse/internal/omnibox"
	"github.com/Grove-Computing/Growse/internal/searchdata"
	"github.com/Grove-Computing/Growse/internal/searchprovider"
	"github.com/Grove-Computing/Growse/internal/ui"
)

//go:embed index.html style.css
var showcaseAssets embed.FS

var showcasePage = template.Must(template.ParseFS(showcaseAssets, "index.html"))

type pageData struct {
	Title string
	Route string
	Query string
}

func searchShowcaseHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/style.css", func(response http.ResponseWriter, _ *http.Request) {
		body, err := fs.ReadFile(showcaseAssets, "style.css")
		if err != nil {
			http.Error(response, "fixture unavailable", http.StatusInternalServerError)
			return
		}
		response.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = response.Write(body)
	})
	mux.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		titles := map[string]string{
			"/tab-guide":      "Guide tab",
			"/tab-ranking":    "Ranking tab",
			"/history-guide":  "Guide history",
			"/bookmark-guide": "Guide bookmark",
			"/search":         "Offline search result",
		}
		title, ok := titles[request.URL.Path]
		if !ok {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := showcasePage.Execute(response, pageData{Title: title, Route: request.URL.Path, Query: request.URL.Query().Get("q")}); err != nil {
			log.Printf("Search Showcase fixture render failed: %v", err)
		}
	})
	return mux
}

type searchShowcase struct {
	server   *httptest.Server
	client   *network.Client
	session  *browser.Session
	chrome   *ui.BrowserUI
	data     *searchdata.Store
	settings searchprovider.Settings
	remote   omnibox.RemoteFetcher
}

func newSearchShowcase(invalidate func()) (*searchShowcase, error) {
	server := httptest.NewTLSServer(searchShowcaseHandler())
	client := network.NewClientWithLimits(server.Client(), 1<<20)
	data := searchdata.NewMemoryStore()
	fixed := time.Date(2026, time.November, 7, 9, 0, 0, 0, time.UTC)
	if err := data.RecordNavigation(searchdata.Navigation{
		URL: server.URL + "/history-guide", Title: "Guide history", VisitedAt: fixed, Typed: true, TopLevel: true, Success: true,
	}); err != nil {
		server.Close()
		return nil, err
	}
	if _, err := data.SaveBookmark("", server.URL+"/bookmark-guide", "Guide bookmark"); err != nil {
		server.Close()
		return nil, err
	}

	settings := searchprovider.Defaults()
	provider := searchprovider.Provider{
		ID: "showcase", Name: "Offline Showcase", Keyword: "demo",
		SearchTemplate: server.URL + "/search?q={searchTerms}",
	}
	if err := settings.Put(provider); err != nil {
		server.Close()
		return nil, err
	}
	settings.DefaultID = provider.ID
	settings.RemoteSuggestions = true
	if err := settings.Validate(); err != nil {
		server.Close()
		return nil, err
	}
	remote := func(ctx context.Context, query string) ([]string, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		return []string{query + " remote", query + " ranking"}, nil
	}

	showcase := &searchShowcase{server: server, client: client, data: data, settings: settings, remote: remote}
	showcase.session = browser.NewSession(func() *browser.Browser {
		state := browser.New(client)
		state.SetNavigationObserver(func(record browser.NavigationRecord) {
			_ = data.RecordNavigation(searchdata.Navigation{
				URL: record.URL, Title: record.Title, Typed: record.Typed, TopLevel: true, Success: true,
			})
			if invalidate != nil {
				invalidate()
			}
		})
		return state
	})
	home, err := showcase.session.NewTab(nil)
	if err != nil {
		showcase.Close()
		return nil, err
	}
	for _, tab := range []struct{ path, title string }{{"/tab-guide", "Guide tab"}, {"/tab-ranking", "Ranking tab"}} {
		if err := showcase.addFixtureTab(tab.path, tab.title); err != nil {
			showcase.Close()
			return nil, err
		}
	}
	if _, err := showcase.session.SelectTab(home.ID); err != nil {
		showcase.Close()
		return nil, err
	}
	showcase.session.SetOnActiveMutation(invalidate)
	showcase.chrome = ui.NewBrowserUIWithTabs(nil, showcase.session, invalidate)
	showcase.chrome.SetSearchDataStore(data)
	if err := showcase.chrome.SetSearchProviders(settings); err != nil {
		showcase.Close()
		return nil, err
	}
	// The Showcase provider is deliberately in-process. No suggestion request
	// reaches the TLS fixture or the public network.
	showcase.chrome.SetSuggestionProvider(remote, true)
	return showcase, nil
}

func (showcase *searchShowcase) addFixtureTab(path, title string) error {
	tab, err := showcase.session.NewTab(nil)
	if err != nil {
		return err
	}
	navigator, ok := showcase.session.BrowserTarget(tab.ID)
	if !ok {
		return fmt.Errorf("fixture tab %d is unavailable", tab.ID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := navigator.Navigate(ctx, showcase.server.URL+path); err != nil {
		return err
	}
	_, err = showcase.session.SetTabTitle(tab.ID, title)
	return err
}

func (showcase *searchShowcase) Close() {
	if showcase == nil {
		return
	}
	if showcase.chrome != nil {
		showcase.chrome.Close()
		showcase.chrome = nil
	}
	if showcase.session != nil {
		_ = showcase.session.Close()
		showcase.session = nil
	}
	if showcase.client != nil {
		showcase.client.FlushCache()
		showcase.client = nil
	}
	if showcase.server != nil {
		showcase.server.Close()
		showcase.server = nil
	}
}

func runSearchShowcase(window *gioapp.Window) error {
	showcase, err := newSearchShowcase(window.Invalidate)
	if err != nil {
		return err
	}
	defer showcase.Close()
	var operations op.Ops
	for {
		switch event := window.Event().(type) {
		case gioapp.DestroyEvent:
			return event.Err
		case gioapp.FrameEvent:
			gtx := gioapp.NewContext(&operations, event)
			showcase.chrome.Layout(gtx)
			event.Frame(gtx.Ops)
		}
	}
}

func main() {
	go func() {
		window := new(gioapp.Window)
		window.Option(
			gioapp.Title("Growse Search Showcase"),
			gioapp.Size(unit.Dp(1280), unit.Dp(800)),
		)
		if err := runSearchShowcase(window); err != nil {
			log.Printf("Growse Search Showcase closed with an error: %v", err)
		}
	}()
	gioapp.Main()
}
