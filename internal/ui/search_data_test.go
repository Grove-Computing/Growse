package ui

import (
	"image"
	"net/url"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/searchdata"
)

func TestBookmarkToolbarActionTogglesCurrentPage(t *testing.T) {
	document := dom.NewDocument()
	title := document.CreateElement("title", nil)
	if err := document.AppendChild(title, document.CreateText("Saved title")); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(document.Root, title); err != nil {
		t.Fatal(err)
	}
	pageURL, err := url.Parse("https://example.com/page")
	if err != nil {
		t.Fatal(err)
	}
	page := browser.NewPage(pageURL)
	page.Document = document
	navigator := &stubNavigator{page: page}
	ui := NewBrowserUI(navigator, nil)
	defer ui.Close()
	store := searchdata.NewMemoryStore()
	ui.SetSearchDataStore(store)

	ui.toggleActiveBookmark()
	bookmark, ok := store.Bookmark(page.URL.String())
	if !ok || bookmark.Title != "Saved title" || ui.status != "Bookmarkへ追加しました" {
		t.Fatalf("bookmark after add = %#v, exists=%v, status=%q", bookmark, ok, ui.status)
	}
	ui.toggleActiveBookmark()
	if _, ok := store.Bookmark(page.URL.String()); ok || ui.status != "Bookmarkを削除しました" {
		t.Fatalf("bookmark still exists=%v, status=%q", ok, ui.status)
	}
}

func TestBookmarkBarListsProfileEntries(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	store := searchdata.NewMemoryStore()
	if _, err := store.SaveBookmark("", "https://one.example/", "One"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveBookmark("", "https://two.example/", "Two"); err != nil {
		t.Fatal(err)
	}
	ui.SetSearchDataStore(store)

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(640, 28)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	dims := ui.layoutBookmarkBar(gtx)
	if got, want := dims.Size.Y, 28; got != want {
		t.Fatalf("bookmark bar height = %d, want %d", got, want)
	}
	if got, want := len(ui.bookmarkBarButtons), 2; got != want {
		t.Fatalf("bookmark bar buttons = %d, want %d", got, want)
	}
}
