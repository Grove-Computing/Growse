package main

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestCSSLayoutShowcaseServesLayoutStagesAndLateImage(t *testing.T) {
	server := httptest.NewServer(cssLayoutHandler())
	defer server.Close()
	for _, route := range []struct {
		path, contentType, marker string
	}{
		{"/", "text/html", "Writing Mode &amp; logical geometry"},
		{"/style.css", "text/css", ".vertical-grid"},
		{"/app.mjs", "text/javascript", "VERTICAL-LR · RTL"},
		{"/assets/late-layout.png", "image/png", ""},
	} {
		response, err := server.Client().Get(server.URL + route.path)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != 200 || !strings.Contains(response.Header.Get("Content-Type"), route.contentType) ||
			route.marker != "" && !strings.Contains(string(body), route.marker) {
			t.Fatalf("GET %s = status:%d type:%q marker:%t err:%v", route.path, response.StatusCode, response.Header.Get("Content-Type"), strings.Contains(string(body), route.marker), readErr)
		}
	}
}

func TestCSSLayoutShowcaseUsesVerticalGlyphAndLogicalAxes(t *testing.T) {
	htmlSource, err := cssLayoutAssets.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	cssSource, err := cssLayoutAssets.ReadFile("style.css")
	if err != nil {
		t.Fatal(err)
	}
	document, err := htmlparser.Parse(strings.NewReader(string(htmlSource)))
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(string(cssSource)))
	if err != nil {
		t.Fatal(err)
	}
	flex, ok := document.QuerySelector(".vertical-flex")
	if !ok || len(flex.Children) == 0 {
		t.Fatal("vertical flex fixture is missing")
	}
	var firstItem = flex.Children[0]
	for _, child := range flex.Children {
		if child.TagName == "span" {
			firstItem = child
			break
		}
	}
	imageNode, ok := document.QuerySelector(".vertical-image")
	if !ok {
		t.Fatal("vertical image fixture is missing")
	}
	images := map[dom.NodeID]layout.ImageResource{
		imageNode.ID: {URL: "/assets/late-layout.png", IntrinsicWidth: 240, IntrinsicHeight: 120, Loaded: true},
	}
	tree := layout.BuildWithScrollAndImages(document, style.Compute(document, stylesheet), images, 1050, 700, 0, 0)
	for _, box := range tree.Boxes {
		if box.NodeID != firstItem.ID || len(box.Runs) < 2 {
			continue
		}
		if box.WritingMode != style.WritingModeVerticalRL || box.Runs[0].OffsetX != box.Runs[1].OffsetX || box.Runs[1].OffsetY <= box.Runs[0].OffsetY {
			t.Fatalf("vertical flex showcase glyph geometry = %#v", box.Runs)
		}
		return
	}
	t.Fatal("vertical flex showcase text box is missing")
}
