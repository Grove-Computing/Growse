package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestIntrinsicKeywordsAndNestedPercentageSizingConverge(t *testing.T) {
	document := dom.NewDocument()
	outer := document.CreateElement("div", map[string]string{"class": "outer"})
	minimum := document.CreateElement("div", map[string]string{"class": "minimum"})
	maximum := document.CreateElement("div", map[string]string{"class": "maximum"})
	fitContainer := document.CreateElement("div", map[string]string{"class": "fit-container"})
	fit := document.CreateElement("div", map[string]string{"class": "fit"})
	host := document.CreateElement("div", map[string]string{"class": "host"})
	grid := document.CreateElement("div", map[string]string{"class": "nested-grid"})
	absolute := document.CreateElement("div", map[string]string{"class": "absolute"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, outer},
		[2]*dom.Node{outer, minimum}, [2]*dom.Node{minimum, document.CreateText("aa bbbbb")},
		[2]*dom.Node{outer, maximum}, [2]*dom.Node{maximum, document.CreateText("aa bbbbb")},
		[2]*dom.Node{outer, fitContainer}, [2]*dom.Node{fitContainer, fit}, [2]*dom.Node{fit, document.CreateText("long phrase for fitting")},
		[2]*dom.Node{outer, host}, [2]*dom.Node{host, grid}, [2]*dom.Node{grid, document.CreateText("grid")},
		[2]*dom.Node{host, absolute}, [2]*dom.Node{absolute, document.CreateText("absolute")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.outer { display:flex; width:600px; gap:10px; align-items:flex-start }
.minimum { flex:0 0 auto; width:min-content; background-color:#ddd }
.maximum { flex:0 0 auto; width:max-content; background-color:#ccc }
.fit-container { flex:0 0 90px; width:90px }
.fit { display:block; width:fit-content; background-color:#bbb }
.host { position:relative; flex:0 0 200px; width:200px; height:80px; background-color:#eee }
.nested-grid { display:grid; width:50%; height:20px; background-color:#abc }
.absolute { position:absolute; left:25%; width:50%; top:30px; height:20px; background-color:#def }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := stylemodel.Compute(document, stylesheet)
	minimumStyle, _ := computed.For(minimum)
	maximumStyle, _ := computed.For(maximum)
	fitStyle, _ := computed.For(fit)
	if minimumStyle.Width.Kind != stylemodel.SizeMinContent || maximumStyle.Width.Kind != stylemodel.SizeMaxContent || fitStyle.Width.Kind != stylemodel.SizeFitContent {
		t.Fatalf("intrinsic size kinds = %v/%v/%v", minimumStyle.Width.Kind, maximumStyle.Width.Kind, fitStyle.Width.Kind)
	}
	tree := Build(document, computed, 800)
	minimumRect := decorationForNode(t, tree, minimum.ID).Rect
	maximumRect := decorationForNode(t, tree, maximum.ID).Rect
	fitRect := decorationForNode(t, tree, fit.ID).Rect
	hostRect := decorationForNode(t, tree, host.ID).Rect
	gridRect := decorationForNode(t, tree, grid.ID).Rect
	absoluteRect := decorationForNode(t, tree, absolute.ID).Rect
	if !(minimumRect.Width > 0 && maximumRect.Width > minimumRect.Width) {
		t.Fatalf("min/max content widths = %v/%v", minimumRect.Width, maximumRect.Width)
	}
	if fitRect.Width > 90 || fitRect.Width < minimumRect.Width {
		t.Fatalf("fit-content width = %v, want clamp between min-content and 90", fitRect.Width)
	}
	if difference := gridRect.Width - hostRect.Width*.5; difference < -.01 || difference > .01 {
		t.Fatalf("nested grid percentage width = %v, host=%v", gridRect.Width, hostRect.Width)
	}
	if difference := absoluteRect.Width - hostRect.Width*.5; difference < -.01 || difference > .01 {
		t.Fatalf("positioned percentage width = %v, host=%v", absoluteRect.Width, hostRect.Width)
	}
	if difference := absoluteRect.X - (hostRect.X + hostRect.Width*.25); difference < -.01 || difference > .01 {
		t.Fatalf("positioned percentage x = %v, host=%#v", absoluteRect.X, hostRect)
	}
}
