package browser

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestBrowserUAProfileIsAppliedOnlyToExplicitJavaScriptPage(t *testing.T) {
	document := dom.NewDocument()
	html := document.CreateElement("html", nil)
	body := document.CreateElement("body", nil)
	input := document.CreateElement("input", nil)
	for _, edge := range [][2]*dom.Node{{document.Root, html}, {html, body}, {body, input}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}

	goPage := &Page{Document: document, Engine: runtimemodel.EngineGo, Compatibility: CompatibilityProfileGo, ViewportWidth: 800, ViewportHeight: 600}
	goPage.ComputedStyles = computePageStyles(goPage)
	goInput, _ := goPage.ComputedStyles.For(input)
	goTree := layout.BuildWithViewport(document, goPage.ComputedStyles, 800, 600)

	jsPage := &Page{Document: document, Engine: runtimemodel.EngineJavaScript, Compatibility: CompatibilityProfileModernWeb, ViewportWidth: 800, ViewportHeight: 600}
	jsPage.ComputedStyles = computePageStyles(jsPage)
	jsInput, _ := jsPage.ComputedStyles.For(input)
	jsTree := layout.BuildWithViewport(document, jsPage.ComputedStyles, 800, 600)

	if goInput.Display == jsInput.Display || goInput.BrowserDefaults || !jsInput.BrowserDefaults {
		t.Fatalf("engine UA split input: go=%#v js=%#v", goInput, jsInput)
	}
	goHTML := goTree.Bounds[html.ID]
	jsHTML := jsTree.Bounds[html.ID]
	if goHTML.X != 32 || jsHTML.X != 0 || goHTML.Width != 736 || jsHTML.Width != 800 {
		t.Fatalf("initial containing block go=%#v js=%#v", goHTML, jsHTML)
	}
	jsBody := jsTree.Bounds[body.ID]
	if jsBody.X != 8 || jsBody.Width != 784 {
		t.Fatalf("browser body margin geometry = %#v, want x=8 width=784", jsBody)
	}
}
