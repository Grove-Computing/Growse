package ui

import (
	"image"
	"net/url"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/Grove-Computing/Growse/internal/browser"
)

func TestEmptyAndCreatedTabsOpenInternalHome(t *testing.T) {
	session := browser.NewSession()
	first, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	ui := NewBrowserUIWithTabs(nil, session, nil)
	ui.syncActiveTabChrome()
	if !ui.homeVisible() {
		t.Fatal("initial empty tab did not open internal home")
	}

	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(1280, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.createTab(gtx)
	active, ok := session.ActiveTab()
	if !ok || active.ID == first.ID || !ui.homeVisible() {
		t.Fatalf("created tab = (%+v, %v), home = %v", active, ok, ui.homeVisible())
	}
}

func TestTabWithExplicitURLDoesNotOpenInternalHome(t *testing.T) {
	session := browser.NewSession()
	target, err := url.Parse("https://example.test/path")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.NewTab(target); err != nil {
		t.Fatal(err)
	}
	ui := NewBrowserUIWithTabs(nil, session, nil)
	ui.syncActiveTabChrome()
	if ui.homeVisible() {
		t.Fatal("URL-initialized tab opened internal home")
	}
}

func TestHomeActionShowsBrowserOwnedViewWithoutReplacingPage(t *testing.T) {
	target, err := url.Parse("https://example.test/")
	if err != nil {
		t.Fatal(err)
	}
	page := browser.NewPage(target)
	navigator := &stubNavigator{page: page}
	ui := NewBrowserUI(navigator, nil)
	ui.setHomeVisible(0, false)

	ui.showHome()

	if !ui.homeVisible() {
		t.Fatal("Home action did not show internal home")
	}
	if navigator.Page() != page {
		t.Fatal("Home action replaced the underlying web page")
	}
	if ui.address.Text() != "" {
		t.Fatalf("Home omnibox = %q, want blank", ui.address.Text())
	}
}

func TestInternalHomeFillsViewport(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(900, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	if dims := ui.layoutViewport(gtx); dims.Size != image.Pt(900, 600) {
		t.Fatalf("home dimensions = %v", dims.Size)
	}
}
