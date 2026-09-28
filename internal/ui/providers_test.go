package ui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/Grove-Computing/Growse/internal/searchprovider"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	if err != nil || got != "https://html.duckduckgo.com/html/?q=gopher" {
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

func providerSettingsFrame(ui *BrowserUI) {
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(700, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.layoutProviderSettings(gtx)
}
func TestProviderSettingsControls(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	providerSettingsFrame(ui)
	p := searchprovider.Provider{ID: "custom", Name: "Example", Keyword: "ex", SearchTemplate: "https://example.com/?q={searchTerms}"}
	ui.selectProvider(p)
	ui.providerPanel.save.Click()
	providerSettingsFrame(ui)
	if len(ui.SearchProviders().Providers) != 2 {
		t.Fatal("UI save did not add provider")
	}
	ui.providerPanel.rows[p.ID].choose.Click()
	providerSettingsFrame(ui)
	if ui.SearchProviders().DefaultID != p.ID {
		t.Fatal("UI default not changed")
	}
	ui.providerPanel.rows["duckduckgo"].disable.Click()
	providerSettingsFrame(ui)
	if !ui.SearchProviders().Providers[0].Disabled {
		t.Fatal("builtin not disabled")
	}
	ui.providerPanel.rows["duckduckgo"].disable.Click()
	providerSettingsFrame(ui)
	ui.selectProvider(searchprovider.Provider{ID: p.ID, Name: "Edited", Keyword: "ex", SearchTemplate: p.SearchTemplate})
	ui.providerPanel.save.Click()
	providerSettingsFrame(ui)
	if ui.SearchProviders().Providers[1].Name != "Edited" {
		t.Fatal("UI edit not saved")
	}
	ui.providerPanel.remote.Click()
	providerSettingsFrame(ui)
	if !ui.SearchProviders().RemoteSuggestions {
		t.Fatal("opt-in failed")
	}
	ui.providerPanel.remote.Click()
	providerSettingsFrame(ui)
	if ui.SearchProviders().RemoteSuggestions {
		t.Fatal("opt-out failed")
	}
	ui.providerPanel.rows[p.ID].remove.Click()
	providerSettingsFrame(ui)
	if len(ui.SearchProviders().Providers) != 1 || ui.SearchProviders().DefaultID != "duckduckgo" {
		t.Fatal("UI delete failed")
	}
}
func TestProviderFailedWriteKeepsLiveSettings(t *testing.T) {
	root := t.TempDir()
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	if err := ui.OpenSearchProfile(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "search-providers.json"), 0700); err != nil {
		t.Fatal(err)
	}
	s := ui.SearchProviders()
	s.RemoteSuggestions = true
	if err := ui.SetSearchProviders(s); err == nil {
		t.Fatal("accepted failed rename")
	}
	if ui.SearchProviders().RemoteSuggestions || ui.remoteSuggestions {
		t.Fatal("failed write changed live settings")
	}
}
func TestKeywordCandidateKeepsProvider(t *testing.T) {
	navigator := &recordingNavigator{navigated: make(chan string, 1)}
	ui := NewBrowserUI(navigator, nil)
	defer ui.Close()
	s := ui.SearchProviders()
	p := searchprovider.Provider{ID: "custom", Name: "Example", Keyword: "ex", SearchTemplate: "https://example.com/?q={searchTerms}"}
	if err := s.Put(p); err != nil {
		t.Fatal(err)
	}
	if err := ui.SetSearchProviders(s); err != nil {
		t.Fatal(err)
	}
	ui.address.SetText("ex gopher")
	ui.refreshSuggestions()
	if ui.suggestionPopup.providerKeyword != "ex" || len(ui.suggestionPopup.candidates) == 0 {
		t.Fatal("keyword not retained")
	}
	ui.submitSuggestion(ui.suggestionPopup.candidates[0], omniboxCurrentTab)
	select {
	case target := <-navigator.navigated:
		if target != "https://example.com/?q=gopher" {
			t.Fatal(target)
		}
	case <-time.After(time.Second):
		t.Fatal("keyword candidate did not navigate")
	}
	if ui.SearchProviders().DefaultID != "duckduckgo" {
		t.Fatal("keyword changed default")
	}
}

func TestOptOutCancelsEvenWhenSaveFails(t *testing.T) {
	root := t.TempDir()
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	if err := ui.OpenSearchProfile(root); err != nil {
		t.Fatal(err)
	}
	s := ui.SearchProviders()
	s.RemoteSuggestions = true
	if err := ui.SetSearchProviders(s); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "search-providers.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	ui.address.SetText("gopher")
	ui.refreshSuggestions()
	before, _ := ui.suggestions.Results()
	s.RemoteSuggestions = false
	if err := ui.SetSearchProviders(s); err == nil {
		t.Fatal("expected write failure")
	}
	after, _ := ui.suggestions.Results()
	if ui.remoteSuggestions || ui.SearchProviders().RemoteSuggestions || after <= before {
		t.Fatal("failed save prevented immediate opt-out")
	}
}
