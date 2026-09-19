package browser

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
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
	if page.StyleRevision != 18 {
		t.Fatalf("font completion render revision = %d, want 18", page.StyleRevision)
	}
	if page.WebFonts == nil || len(page.Fonts) != 1 {
		t.Fatal("decoded font was not installed")
	}
	wantAncestors := []dom.NodeID{matchingText.ID, matching.ID, document.Root.ID}
	if !equalNodeIDs(invalidation.LayoutAncestors, wantAncestors) {
		t.Fatalf("font layout ancestors = %v, want %v", invalidation.LayoutAncestors, wantAncestors)
	}
	renderDirty := page.RenderInvalidationSnapshot()
	if renderDirty.Revision != page.StyleRevision || renderDirty.Damage != RenderDamageLayout {
		t.Fatalf("font render invalidation = %#v", renderDirty)
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

func TestCommitWebFontCompletionRejectsStaleGeneration(t *testing.T) {
	face, err := textfont.ParseTTF(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatal(err)
	}
	page := &Page{StyleRevision: 9}
	stale := page.BeginWebFontLoad()
	current := page.BeginWebFontLoad()
	resource := FontResource{Family: "Fixture", URL: "https://example.com/fixture.woff2", Loaded: true, Decoded: true, Face: face}
	if invalidation, committed := page.CommitWebFontCompletionForGeneration(stale, resource); committed || invalidation.Revision != 0 {
		t.Fatalf("stale font completion was committed: %#v", invalidation)
	}
	if len(page.Fonts) != 0 || page.StyleRevision != 9 {
		t.Fatalf("stale font completion changed page: fonts=%d revision=%d", len(page.Fonts), page.StyleRevision)
	}
	if _, committed := page.CommitWebFontCompletionForGeneration(current, resource); !committed || len(page.Fonts) != 1 {
		t.Fatal("current font completion was rejected")
	}
}

func TestWebFontSwapRelayoutKeepsFallbackTextAndFollowingContentVisible(t *testing.T) {
	document := dom.NewDocument()
	heading := document.CreateElement("h1", map[string]string{"class": "heading"})
	copy := document.CreateElement("p", map[string]string{"class": "copy"})
	headingText := document.CreateText("iiiiiiiiiiii 日本語")
	for _, edge := range [][2]*dom.Node{
		{document.Root, heading}, {heading, headingText},
		{document.Root, copy}, {copy, document.CreateText("fallback copy remains visible")},
	} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.heading { display:block; width:150px; margin:0 0 6px; font:24px/30px Fixture,sans-serif; overflow-wrap:anywhere }
.copy { display:block; width:150px; margin:0; font:16px/24px sans-serif }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.Compute(document, stylesheet)
	page := &Page{
		Document: document, ComputedStyles: computed, Compatibility: CompatibilityProfileModernWeb,
		WebFonts: layoutmodel.NewFontSetWithSystemFallback(nil), StyleRevision: 4,
	}
	fallback := layoutmodel.BuildWithScrollAndResources(document, computed, nil, page.WebFonts, 240, 400, 0, 0)
	face, err := textfont.ParseTTF(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatal(err)
	}
	invalidation := page.CommitWebFontCompletion(FontResource{
		Family: "Fixture", Style: "normal", Weight: "normal", Loaded: true, Decoded: true, Face: face,
		UnicodeRanges: []FontRange{{Start: 0x20, End: 0x7f}},
	})
	loaded := layoutmodel.BuildWithScrollAndResources(document, computed, nil, page.WebFonts, 240, 400, 0, 0)
	if invalidation.Revision == 0 || page.StyleRevision != 5 {
		t.Fatalf("font swap did not invalidate render state: invalidation=%#v revision=%d", invalidation, page.StyleRevision)
	}
	for name, tree := range map[string]*layoutmodel.Tree{"fallback": fallback, "loaded": loaded} {
		headingRect, copyRect := tree.Bounds[heading.ID], tree.Bounds[copy.ID]
		if headingRect.Height <= 0 || copyRect.Height <= 0 || copyRect.Y < headingRect.Y+headingRect.Height {
			t.Fatalf("%s font state clipped or overlapped content: heading=%#v copy=%#v", name, headingRect, copyRect)
		}
		visible := ""
		for _, box := range tree.Boxes {
			visible += box.Text
		}
		for _, want := range []string{"iiiiiiiiiiii", "日本語", "fallbackcopyremainsvisible"} {
			if !strings.Contains(strings.ReplaceAll(visible, " ", ""), want) {
				t.Fatalf("%s font state lost %q from %q", name, want, visible)
			}
		}
	}
}
