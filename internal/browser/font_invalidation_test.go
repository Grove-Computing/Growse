package browser

import (
	"bytes"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
	textfont "github.com/go-text/typesetting/font"
	"golang.org/x/image/font/gofont/goregular"
)

func TestCommitWebFontCompletionInvalidatesOnlyCoveredGlyphRuns(t *testing.T) {
	document := dom.NewDocument()
	matching := document.CreateElement("p", nil)
	unrelated := document.CreateElement("p", nil)
	matchingText := document.CreateText("ABCΩDEF")
	unrelatedText := document.CreateText("browser chrome")
	for _, edge := range [][2]*dom.Node{{document.Root, matching}, {matching, matchingText}, {document.Root, unrelated}, {unrelated, unrelatedText}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	face, err := textfont.ParseTTF(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatal(err)
	}
	page := &Page{
		Document: document,
		ComputedStyles: style.Map{
			matching.ID:  {FontFamilies: []string{"Fixture", "sans-serif"}},
			unrelated.ID: {FontFamilies: []string{"Growse UI", "sans-serif"}},
		},
		Compatibility: CompatibilityProfileModernWeb,
		StyleRevision: 17,
	}
	resource := FontResource{
		Family: "Fixture", Style: "normal", Weight: "normal", Stretch: "normal",
		URL: "https://example.com/fixture.woff2", Loaded: true, Decoded: true, Face: face,
		UnicodeRanges: []FontRange{{Start: 'A', End: 'Z'}},
	}
	invalidation := page.CommitWebFontCompletion(resource)
	want := []GlyphRunInvalidation{
		{TextNode: matchingText.ID, RuneStart: 0, RuneEnd: 3},
		{TextNode: matchingText.ID, RuneStart: 4, RuneEnd: 7},
	}
	if invalidation.Revision != 1 || len(invalidation.Runs) != len(want) {
		t.Fatalf("invalidation = %#v, want %#v", invalidation, want)
	}
	for index := range want {
		if invalidation.Runs[index] != want[index] {
			t.Fatalf("run %d = %#v, want %#v", index, invalidation.Runs[index], want[index])
		}
	}
	if page.StyleRevision != 17 {
		t.Fatalf("font completion rebuilt page styles: revision=%d", page.StyleRevision)
	}
	if page.WebFonts == nil || len(page.Fonts) != 1 {
		t.Fatal("decoded font was not installed")
	}

	snapshot := page.FontInvalidationSnapshot()
	snapshot.Runs[0].RuneStart = 99
	if page.FontInvalidationSnapshot().Runs[0].RuneStart != 0 {
		t.Fatal("font invalidation snapshot aliases page state")
	}
}

func TestCommitWebFontCompletionIgnoresFailedFont(t *testing.T) {
	page := &Page{}
	if got := page.CommitWebFontCompletion(FontResource{Family: "Fixture", Error: "decode failed"}); got.Revision != 0 || len(page.Fonts) != 0 {
		t.Fatalf("failed font changed page: %#v", got)
	}
}
