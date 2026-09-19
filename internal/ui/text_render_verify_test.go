package ui

import (
	"image"
	"strings"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/html"
	layoutengine "github.com/Grove-Computing/Growse/internal/layout"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestTextShaperInitialization(t *testing.T) {
	htmlContent := `<!doctype html>
<html>
<head>
  <style>
    body { font-size: 16px; color: #111827; background: #ffffff; margin: 20px; }
    h1 { color: #2563eb; font-size: 24px; margin-bottom: 12px; }
    p { font-size: 16px; line-height: 1.5; color: #374151; }
    .jp-box { font-size: 18px; color: #059669; margin-top: 10px; }
    .vertical-box { writing-mode: vertical-rl; font-size: 16px; color: #dc2626; border: 1px solid #fca5a5; padding: 8px; }
  </style>
</head>
<body>
  <h1>Growse Text Rendering Fix Verification</h1>
  <p>This is a test of horizontal text rendering in Growse browser engine.</p>
  <div class="jp-box">日本語のテキストが正常にレンダリングされることを確認します。</div>
  <div class="vertical-box">縦書き表示テスト</div>
</body>
</html>`

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("HTML parse error: %v", err)
	}

	stylesheet, _ := css.Parse(strings.NewReader(extractTestStyles(doc)))
	computed := stylemodel.Compute(doc, stylesheet)
	tree := layoutengine.Build(doc, computed, 800)
	displayList := paintmodel.Build(tree)

	if len(displayList.Commands) == 0 {
		t.Fatalf("DisplayList has no commands")
	}

	drawTextCount := 0
	for _, cmd := range displayList.Commands {
		if textCmd, ok := cmd.(paintmodel.DrawText); ok {
			drawTextCount++
			if textCmd.Text == "" && len(textCmd.Runs) == 0 {
				t.Errorf("DrawText command has empty text and empty runs: %#v", textCmd)
			}
		}
	}

	if drawTextCount == 0 {
		t.Fatalf("No DrawText commands generated")
	}

	ui := NewBrowserUI(nil, nil)
	gtx := layout.Context{
		Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(1000, 700)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	page := &browser.Page{
		Document: doc, ComputedStyles: computed, StyleRevision: 1,
	}

	// Execute layoutDocument to trigger installPageFonts and layoutDrawText
	dimensions := ui.layoutDocument(gtx, page)
	if dimensions.Size.X <= 0 || dimensions.Size.Y <= 0 {
		t.Fatalf("layoutDocument returned invalid dimensions: %v", dimensions.Size)
	}

	// Verify that documentTheme().Shaper is valid and non-nil
	shaper := ui.documentTheme().Shaper
	if shaper == nil {
		t.Fatalf("documentTheme().Shaper is nil after layoutDocument")
	}

}

func extractTestStyles(doc *dom.Document) string {
	var styles strings.Builder
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "style" {
			styles.WriteString(node.TextContent())
			styles.WriteString("\n")
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(doc.Root)
	return styles.String()
}
