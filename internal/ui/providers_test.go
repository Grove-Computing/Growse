package ui

import (
	"github.com/Grove-Computing/Growse/internal/searchprovider"
	"testing"
)

func TestProviderSearchSelection(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	s := ui.SearchProviders()
	p := searchprovider.Provider{ID: "custom", Name: "Custom", Keyword: "c", SearchTemplate: "https://example.com/?q={searchTerms}"}
	if err := s.Put(p); err != nil {
		t.Fatal(err)
	}
	s.DefaultID = p.ID
	if err := ui.SetSearchProviders(s); err != nil {
		t.Fatal(err)
	}
	got, err := ui.providerSearchURL("ddg gopher")
	if err != nil || got != "https://duckduckgo.com/?q=gopher" {
		t.Fatal(got, err)
	}
	got, err = ui.providerSearchURL("gopher")
	if err != nil || got != "https://example.com/?q=gopher" {
		t.Fatal(got, err)
	}
	if ui.SearchProviders().DefaultID != "custom" {
		t.Fatal("keyword changed default")
	}
	s.Providers[0].Name = "mutated"
	if ui.SearchProviders().Providers[0].Name != "DuckDuckGo" {
		t.Fatal("aliased settings")
	}
}

func TestProviderPrivacyDefaultsAndDisable(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	if ui.SearchProviders().RemoteSuggestions || ui.remoteSuggestions {
		t.Fatal("suggestions enabled by default")
	}
	s := ui.SearchProviders()
	s.RemoteSuggestions = true
	if err := ui.SetSearchProviders(s); err != nil {
		t.Fatal(err)
	}
	ui.address.SetText("gopher")
	ui.refreshSuggestions()
	generation, _ := ui.suggestions.Results()
	s.RemoteSuggestions = false
	if err := ui.SetSearchProviders(s); err != nil {
		t.Fatal(err)
	}
	after, results := ui.suggestions.Results()
	if after <= generation || len(results) != 0 || ui.remoteSuggestions || ui.suggestionPopup.open {
		t.Fatal("disable retained generation")
	}
}

func TestProviderProfileRestartAndFailedCommit(t *testing.T) {
	root := t.TempDir()
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	if err := ui.OpenSearchProfile(root); err != nil {
		t.Fatal(err)
	}
	s := ui.SearchProviders()
	p := searchprovider.Provider{ID: "custom", Name: "Custom", Keyword: "c", SearchTemplate: "https://example.com/?q={searchTerms}"}
	if err := s.Put(p); err != nil {
		t.Fatal(err)
	}
	s.DefaultID = p.ID
	if err := ui.SetSearchProviders(s); err != nil {
		t.Fatal(err)
	}
	second := NewBrowserUI(nil, nil)
	defer second.Close()
	if err := second.OpenSearchProfile(root); err != nil {
		t.Fatal(err)
	}
	if second.SearchProviders().DefaultID != "custom" {
		t.Fatal("restart lost settings")
	}
	// An invalid configuration cannot commit or change live state.
	s.DefaultID = "missing"
	if ui.SetSearchProviders(s) == nil || ui.SearchProviders().DefaultID != "custom" {
		t.Fatal("invalid commit changed state")
	}
}
