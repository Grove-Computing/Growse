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
	original := ui.theme.Shaper
	page := &browser.Page{Engine: runtimemodel.EngineJavaScript, StyleRevision: 1, Fonts: []browser.FontResource{{
		Family: "Fixture", Style: "normal", Weight: "normal", Decoded: true, Face: face,
	}}}
	ui.installPageFonts(page)
	if ui.theme.Shaper == nil || ui.theme.Shaper == original || ui.fontPage != page || ui.fontRevision != 1 {
		t.Fatalf("font shaper state = shaper:%p page:%p revision:%d", ui.theme.Shaper, ui.fontPage, ui.fontRevision)
	}
	installed := ui.theme.Shaper
	ui.installPageFonts(page)
	if ui.theme.Shaper != installed {
		t.Fatal("unchanged font revision rebuilt the shaper")
	}

	fallbackPage := &browser.Page{Engine: runtimemodel.EngineJavaScript, StyleRevision: 1, Fonts: []browser.FontResource{{Family: "Broken", Error: "font load timed out"}}}
	ui.installPageFonts(fallbackPage)
	if ui.theme.Shaper == nil || ui.theme.Shaper == installed {
		t.Fatal("font failure did not install the bundled fallback collection")
	}
	goPage := &browser.Page{Engine: runtimemodel.EngineGo, StyleRevision: 1}
	ui.installPageFonts(goPage)
	if ui.theme.Shaper == nil || ui.theme.Shaper == installed || ui.fontPage != goPage {
		t.Fatal("Go Engine did not retain its independent default font path")
	}
}
