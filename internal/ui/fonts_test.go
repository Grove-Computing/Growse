package ui

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/Grove-Computing/Growse/internal/browser"
	layoutengine "github.com/Grove-Computing/Growse/internal/layout"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	textfont "github.com/go-text/typesetting/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/math/fixed"
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

func TestModernWebPageShaperUsesSystemCJKGlyphFallback(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	ui := &BrowserUI{theme: material.NewTheme()}
	chromeShaper := ui.theme.Shaper
	page := &browser.Page{
		Engine: runtimemodel.EngineJavaScript, Compatibility: browser.CompatibilityProfileModernWeb, StyleRevision: 1,
	}
	ui.installPageFonts(page)
	ui.pageTheme.Shaper.LayoutString(text.Parameters{
		Font: font.Font{Typeface: "system-ui"}, PxPerEm: fixed.I(16), MaxLines: 1, MaxWidth: 4096,
	}, "日本語")
	glyphs := make(map[text.GlyphID]bool)
	for {
		glyph, ok := ui.pageTheme.Shaper.NextGlyph()
		if !ok {
			break
		}
		if glyph.Runes != 0 {
			glyphs[glyph.ID] = true
		}
	}
	if len(glyphs) < 3 {
		t.Skipf("system does not provide distinct Japanese glyphs: %#v", glyphs)
	}
	if ui.theme.Shaper != chromeShaper {
		t.Fatal("CJK Page shaping mutated the Browser chrome shaper")
	}

	goPage := &browser.Page{Engine: runtimemodel.EngineGo, Compatibility: browser.CompatibilityProfileGo, StyleRevision: 1}
	ui.installPageFonts(goPage)
	if ui.pageTheme.Shaper == nil || ui.theme.Shaper != chromeShaper {
		t.Fatal("Go Page did not return to its isolated default shaper")
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

func TestPageShaperCacheFollowsPageGenerationAndFontRevision(t *testing.T) {
	ui := &BrowserUI{theme: material.NewTheme()}
	firstPage := &browser.Page{
		Engine: runtimemodel.EngineJavaScript, Compatibility: browser.CompatibilityProfileModernWeb, StyleRevision: 7,
	}
	ui.installPageFonts(firstPage)
	firstShaper := ui.pageTheme.Shaper
	ui.installPageFonts(firstPage)
	if ui.pageTheme.Shaper != firstShaper {
		t.Fatal("same Page generation and font revision replaced the bounded Gio shaping cache")
	}

	firstPage.StyleRevision++
	ui.installPageFonts(firstPage)
	revisedShaper := ui.pageTheme.Shaper
	if revisedShaper == firstShaper {
		t.Fatal("font-affecting Style revision retained a stale shaping cache")
	}

	secondPage := &browser.Page{
		Engine: runtimemodel.EngineJavaScript, Compatibility: browser.CompatibilityProfileModernWeb, StyleRevision: firstPage.StyleRevision,
	}
	ui.installPageFonts(secondPage)
	if ui.pageTheme.Shaper == revisedShaper || ui.fontPage != secondPage {
		t.Fatal("Navigation reused the previous Page generation shaping cache")
	}
}

func TestPageShaperTerminatesWithBundledFallbackWhenSystemFontsAreUnavailable(t *testing.T) {
	shaper := newPageTextShaper(gofont.Collection(), false)
	textValue := "日本語\U0010ffff"
	shaper.LayoutString(text.Parameters{
		Font: font.Font{Typeface: "system-ui"}, PxPerEm: fixed.I(16), MaxLines: 1, MaxWidth: 4096,
	}, textValue)
	runes, glyphs := 0, 0
	for {
		glyph, ok := shaper.NextGlyph()
		if !ok {
			break
		}
		glyphs++
		runes += int(glyph.Runes)
		if glyphs > len([]rune(textValue))*4+8 {
			t.Fatal("missing-glyph fallback did not terminate")
		}
	}
	if runes != len([]rune(textValue)) || glyphs == 0 {
		t.Fatalf("finite fallback = runes:%d glyphs:%d", runes, glyphs)
	}
}

func TestLayoutTextRunPreservesLayoutAdvanceAcrossFontFallback(t *testing.T) {
	ui := &BrowserUI{theme: material.NewTheme()}
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(200, 24)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	dimensions := ui.layoutTextRun(gtx, paintmodel.TextRun{
		Text: "日本語", FontSize: 16, Width: 73, Baseline: 18, Color: 0x111827ff,
	}, 24)
	if dimensions.Size != image.Pt(73, 24) {
		t.Fatalf("text run dimensions = %v, want layout advance 73x24", dimensions.Size)
	}
	if dimensions.Baseline != 6 {
		t.Fatalf("text run baseline from bottom = %d, want CSS baseline 18px from top in a 24px line", dimensions.Baseline)
	}
}

func TestLayoutDecoratedLabelDoesNotConstrainGlyphInkToCSSLineHeight(t *testing.T) {
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(120, 24)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	measured := layout.Constraints{}
	dimensions := layoutDecoratedLabel(gtx, func(gtx layout.Context) layout.Dimensions {
		measured = gtx.Constraints
		return layout.Dimensions{Size: image.Pt(80, 30), Baseline: 7}
	}, 0, 0, 18, 16)
	if measured.Min.X != 0 || measured.Max.X < 1<<20 || measured.Min.Y != 0 || measured.Max.Y < 48 {
		t.Fatalf("glyph measurement constraints = %v, want relaxed height beyond the 24px CSS line", measured)
	}
	if dimensions.Size != image.Pt(80, 24) || dimensions.Baseline != 6 {
		t.Fatalf("baseline-aligned line dimensions = %v baseline:%d, want 80x24 baseline:6", dimensions.Size, dimensions.Baseline)
	}
}

func TestLayoutDrawTextRasterKeepsGlyphInkOutsideTightLineBox(t *testing.T) {
	const width, height = 120, 80
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless raster backend unavailable: %v", err)
	}
	defer window.Release()

	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Constraints: layout.Constraints{Max: image.Pt(width, height)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	offset := op.Offset(image.Pt(0, 30)).Push(ops)
	layoutTextCursorArea(gtx, 80, 8, 0, func(gtx layout.Context) layout.Dimensions {
		// Model glyph ink with an ascent/descent larger than line-height.
		paint.FillShape(gtx.Ops, rgba(0xff0000ff), clip.Rect{Min: image.Pt(10, -20), Max: image.Pt(40, 30)}.Op())
		return layout.Dimensions{Size: image.Pt(80, 8), Baseline: 2}
	})
	offset.Pop()
	if err := window.Frame(ops); err != nil {
		t.Fatal(err)
	}
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(result); err != nil {
		t.Fatal(err)
	}

	outsideInk, totalInk := 0, 0
	minInkY, maxInkY := height, -1
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixel := result.RGBAAt(x, y)
			if pixel.A != 0 && (pixel.R != 0 || pixel.G != 0 || pixel.B != 0) {
				totalInk++
				minInkY, maxInkY = min(minInkY, y), max(maxInkY, y)
				if y < 30 || y >= 38 {
					outsideInk++
				}
			}
		}
	}
	if outsideInk == 0 {
		t.Fatalf("glyph ink was clipped to the CSS line box: total=%d y=%d..%d", totalInk, minInkY, maxInkY)
	}
}

func TestDocumentPaintLayerKeepsBacktrackedSiblingVisible(t *testing.T) {
	const width, height = 240, 180
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless raster backend unavailable: %v", err)
	}
	defer window.Release()

	ui := &BrowserUI{theme: material.NewTheme()}
	page := &browser.Page{}
	displayList := &paintmodel.DisplayList{
		Height: 800, ScrollHeight: 800,
		Commands: []paintmodel.Command{
			paintmodel.DrawBox{NodeID: 1, X: 0, Y: 0, Top: 0, Width: 90, Height: 700, Color: 0x2563ebff, Opacity: 1},
			// This sibling is later in paint order but backtracks near the top,
			// matching a tall left card followed by a right-hand grid card.
			paintmodel.DrawBox{NodeID: 2, X: 120, Y: 30, Top: -670, Width: 80, Height: 60, Color: 0xdc2626ff, Opacity: 1},
		},
	}
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops: ops, Constraints: layout.Exact(image.Pt(width, height)),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	material.List(ui.documentTheme(), &ui.pageList).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return ui.layoutDocumentPaintLayer(gtx, displayList, page)
	})
	if err := window.Frame(ops); err != nil {
		t.Fatal(err)
	}
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(result); err != nil {
		t.Fatal(err)
	}
	pixel := result.RGBAAt(150, 50)
	if pixel.R < 180 || pixel.G > 80 || pixel.B > 80 {
		t.Fatalf("backtracked right sibling pixel = %#v, want red paint", pixel)
	}
}

func TestDocumentPaintLayerDrawsReplacedImageAtDocumentCoordinates(t *testing.T) {
	const width, height = 240, 180
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless raster backend unavailable: %v", err)
	}
	defer window.Release()

	source := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			source.SetNRGBA(x, y, color.NRGBA{R: 240, A: 255})
		}
	}
	const sourceURL = "https://example.com/image.png"
	page := &browser.Page{Images: map[string]image.Image{sourceURL: source}}
	ui := &BrowserUI{theme: material.NewTheme()}
	ui.imagePaintCache.prepare(page)
	displayList := &paintmodel.DisplayList{Height: height, ScrollHeight: height, Commands: []paintmodel.Command{
		paintmodel.DrawImage{
			NodeID: 1, URL: sourceURL, X: 80, Y: 40, Top: -700, Width: 60, Height: 50,
			ImageRect: layoutengine.Rect{X: 80, Y: 40, Width: 60, Height: 50},
			ImageClip: layoutengine.Rect{X: 80, Y: 40, Width: 60, Height: 50}, Opacity: 1,
		},
	}}
	ops := new(op.Ops)
	gtx := layout.Context{Ops: ops, Constraints: layout.Exact(image.Pt(width, height)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	material.List(ui.documentTheme(), &ui.pageList).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return ui.layoutDocumentPaintLayer(gtx, displayList, page)
	})
	if err := window.Frame(ops); err != nil {
		t.Fatal(err)
	}
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(result); err != nil {
		t.Fatal(err)
	}
	pixel := result.RGBAAt(100, 60)
	if pixel.R < 180 || pixel.G > 80 || pixel.B > 80 {
		t.Fatalf("replaced image pixel = %#v, want red paint", pixel)
	}
}

func TestFailedImageAltTextIsClippedToImageBounds(t *testing.T) {
	const width, height = 240, 120
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless raster backend unavailable: %v", err)
	}
	defer window.Release()

	ui := &BrowserUI{theme: material.NewTheme()}
	page := &browser.Page{}
	displayList := &paintmodel.DisplayList{Height: height, ScrollHeight: height, Commands: []paintmodel.Command{
		paintmodel.DrawBox{NodeID: 1, X: 0, Y: 0, Width: width, Height: height, Color: 0xffffffff, Opacity: 1},
		paintmodel.DrawImage{NodeID: 2, X: 80, Y: 40, Width: 32, Height: 20, Alt: "Wikipedia wordmark that must not escape", Color: 0x000000ff, Failed: true, Opacity: 1},
	}}
	ops := new(op.Ops)
	gtx := layout.Context{Ops: ops, Constraints: layout.Exact(image.Pt(width, height)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	material.List(ui.documentTheme(), &ui.pageList).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return ui.layoutDocumentPaintLayer(gtx, displayList, page)
	})
	if err := window.Frame(ops); err != nil {
		t.Fatal(err)
	}
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(result); err != nil {
		t.Fatal(err)
	}
	for y := 40; y < 70; y++ {
		for x := 112; x < 220; x++ {
			pixel := result.RGBAAt(x, y)
			if pixel.R < 245 || pixel.G < 245 || pixel.B < 245 {
				t.Fatalf("alt text escaped image bounds at (%d,%d): %#v", x, y, pixel)
			}
		}
	}
}

func TestContainsEmojiPresentationRecognizesSymbolsAndFlags(t *testing.T) {
	for _, value := range []string{"📝", "🇺🇸 English", "☀️"} {
		if !containsEmojiPresentation(value) {
			t.Fatalf("containsEmojiPresentation(%q) = false", value)
		}
	}
	for _, value := range []string{"English", "日本語", "→"} {
		if containsEmojiPresentation(value) {
			t.Fatalf("containsEmojiPresentation(%q) = true", value)
		}
	}
}
