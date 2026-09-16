package layout

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/style"
)

// BenchmarkGridDashboardLayout measures one complete layout pass for a
// representative dashboard-sized explicit Grid with nested card content.
func BenchmarkGridDashboardLayout(b *testing.B) {
	document, computed := benchmarkGridDocument(b, 48)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = BuildWithViewport(document, computed, 1280, 720)
	}
}

// BenchmarkCSSLayout2026Showcase measures one full pass of the release
// showcase, including Flow, Table, Flex, Grid/Subgrid, vertical writing,
// positioned overflow, and multi-column fragmentation.
func BenchmarkCSSLayout2026Showcase(b *testing.B) {
	markup, err := os.ReadFile("../../examples/css-layout-2026/index.html")
	if err != nil {
		b.Fatal(err)
	}
	stylesheetSource, err := os.ReadFile("../../examples/css-layout-2026/style.css")
	if err != nil {
		b.Fatal(err)
	}
	document, err := htmlparser.Parse(strings.NewReader(string(markup)))
	if err != nil {
		b.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(string(stylesheetSource)))
	if err != nil {
		b.Fatal(err)
	}
	computed := style.ComputeWithEnvironment(document, stylesheet, style.InteractionState{}, style.Environment{
		ViewportWidth: 1280, ViewportHeight: 900, RootFontSize: 16, ResolutionDPI: 96, BrowserDefaults: true,
	})
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = BuildWithViewport(document, computed, 1280, 900)
	}
}

func benchmarkGridDocument(tb testing.TB, count int) (*dom.Document, style.Map) {
	tb.Helper()
	document := dom.NewDocument()
	dashboard := document.CreateElement("main", map[string]string{"class": "dashboard"})
	if err := document.AppendChild(document.Root, dashboard); err != nil {
		tb.Fatal(err)
	}
	for index := 0; index < count; index++ {
		card := document.CreateElement("article", map[string]string{"class": "card"})
		label := document.CreateElement("span", map[string]string{"class": "label"})
		if err := document.AppendChild(dashboard, card); err != nil {
			tb.Fatal(err)
		}
		if err := document.AppendChild(card, label); err != nil {
			tb.Fatal(err)
		}
		if err := document.AppendChild(label, document.CreateText(fmt.Sprintf("Metric %02d", index))); err != nil {
			tb.Fatal(err)
		}
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.dashboard { display:grid; width:1152px; grid-template-columns:repeat(6, 1fr); grid-auto-rows:96px; gap:12px; background-color:#eef2ff }
.card { display:grid; grid-template-rows:auto 1fr; padding:12px; border-radius:10px; background-color:#fff; box-shadow:0 4px 12px rgba(15,23,42,.08) }
.label { color:#334155; transform:translateX(2px) }
`))
	if err != nil {
		tb.Fatal(err)
	}
	return document, style.Compute(document, stylesheet)
}
