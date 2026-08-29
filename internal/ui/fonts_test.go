package ui

import (
	"bytes"
	"testing"

	"gioui.org/widget/material"
	"github.com/Grove-Computing/Growse/internal/browser"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	textfont "github.com/go-text/typesetting/font"
	"golang.org/x/image/font/gofont/goregular"
)

func TestInstallPageFontsUsesDecodedFaceAndKeepsBundledFallback(t *testing.T) {
	face, err := textfont.ParseTTF(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatal(err)
	}
	ui := &BrowserUI{theme: material.NewTheme()}
	chromeShaper := ui.theme.Shaper
	page := &browser.Page{Engine: runtimemodel.EngineJavaScript, Compatibility: browser.CompatibilityProfileModernWeb, StyleRevision: 1, Fonts: []browser.FontResource{{
		Family: "Fixture", Style: "normal", Weight: "normal", Decoded: true, Face: face,
	}}}
	ui.installPageFonts(page)
	if ui.pageTheme == nil || ui.pageTheme.Shaper == nil || ui.pageTheme.Shaper == chromeShaper || ui.fontPage != page || ui.fontRevision != 1 {
		t.Fatalf("font shaper state = page theme:%p shaper:%p page:%p revision:%d", ui.pageTheme, ui.pageTheme.Shaper, ui.fontPage, ui.fontRevision)
	}
	if ui.theme.Shaper != chromeShaper {
		t.Fatal("installing Page fonts replaced the Browser chrome shaper")
	}
	installed := ui.pageTheme.Shaper
	ui.installPageFonts(page)
	if ui.pageTheme.Shaper != installed {
		t.Fatal("unchanged font revision rebuilt the shaper")
	}

	fallbackPage := &browser.Page{Engine: runtimemodel.EngineJavaScript, Compatibility: browser.CompatibilityProfileModernWeb, StyleRevision: 1, Fonts: []browser.FontResource{{Family: "Broken", Error: "font load timed out"}}}
	ui.installPageFonts(fallbackPage)
	if ui.pageTheme.Shaper == nil || ui.pageTheme.Shaper == installed {
		t.Fatal("font failure did not install the bundled fallback collection")
	}
	pageFallback := ui.pageTheme.Shaper
	goPage := &browser.Page{Engine: runtimemodel.EngineGo, StyleRevision: 1}
	ui.installPageFonts(goPage)
	if ui.pageTheme.Shaper == nil || ui.pageTheme.Shaper == pageFallback || ui.fontPage != goPage {
		t.Fatal("Go Engine did not retain its independent default font path")
	}
	if ui.theme.Shaper != chromeShaper {
		t.Fatal("Engine switching mutated the Browser chrome shaper")
	}
}

func TestPageFontShapersAreIsolatedAcrossBrowserUIInstances(t *testing.T) {
	first := &BrowserUI{theme: material.NewTheme()}
	second := &BrowserUI{theme: material.NewTheme()}
	firstChrome, secondChrome := first.theme.Shaper, second.theme.Shaper
	page := &browser.Page{Engine: runtimemodel.EngineJavaScript, Compatibility: browser.CompatibilityProfileModernWeb, StyleRevision: 1}
	first.installPageFonts(page)
	second.installPageFonts(page)
	if first.pageTheme == second.pageTheme || first.pageTheme.Shaper == second.pageTheme.Shaper {
		t.Fatal("Browser UI instances shared a Page theme or shaper")
	}
	if first.theme.Shaper != firstChrome || second.theme.Shaper != secondChrome {
		t.Fatal("Page shaper installation changed a chrome shaper")
	}
}
