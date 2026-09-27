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
