package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestNestedPositionedContainingBlocksResolveAutoOpposingInsetsAndPercentages(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	outer := document.CreateElement("div", map[string]string{"class": "outer"})
	relative := document.CreateElement("div", map[string]string{"class": "relative"})
	fill := document.CreateElement("div", map[string]string{"class": "fill"})
	host := document.CreateElement("div", map[string]string{"class": "host"})
	nested := document.CreateElement("div", map[string]string{"class": "nested"})
	transformed := document.CreateElement("div", map[string]string{"class": "transformed"})
	fixed := document.CreateElement("div", map[string]string{"class": "fixed"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, outer},
		[2]*dom.Node{outer, relative},
		[2]*dom.Node{outer, fill},
		[2]*dom.Node{outer, host},
		[2]*dom.Node{host, nested},
		[2]*dom.Node{outer, transformed},
		[2]*dom.Node{transformed, fixed},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.outer { position:relative; width:300px; height:200px; padding:20px; border:5px solid #111; background:#eee }
.relative { position:relative; left:10%; right:5%; top:10%; width:50px; height:20px; background:#aaa }
.fill { position:absolute; left:10px; right:20px; top:10px; bottom:20px; background:#bbb }
.host { position:absolute; left:50px; top:60px; width:100px; height:80px; padding:10px; background:#ccc }
.nested { position:absolute; left:10%; right:20%; top:25%; bottom:25%; background:#ddd }
.transformed { position:absolute; left:180px; top:80px; width:120px; height:80px; padding:10px; transform:translateX(0); background:#999 }
.fixed { position:fixed; left:25%; top:25%; width:25%; height:50%; background:#777 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, style.Compute(document, stylesheet), 640, 480)
	outerRect := tree.Bounds[outer.ID]
	relativeRect := tree.Bounds[relative.ID]
	if relativeRect.X != outerRect.X+25+30 || relativeRect.Y != outerRect.Y+25+20 {
		t.Fatalf("relative percentage geometry = outer:%#v relative:%#v", outerRect, relativeRect)
	}
	fillRect := tree.Bounds[fill.ID]
	if fillRect != (Rect{X: outerRect.X + 5 + 10, Y: outerRect.Y + 5 + 10, Width: 310, Height: 210}) {
		t.Fatalf("opposing inset fill = %#v", fillRect)
	}
	hostRect, nestedRect := tree.Bounds[host.ID], tree.Bounds[nested.ID]
	if nestedRect != (Rect{X: hostRect.X + 12, Y: hostRect.Y + 25, Width: 84, Height: 50}) {
		t.Fatalf("nested percentage fill = host:%#v nested:%#v", hostRect, nestedRect)
	}
	transformedRect, fixedRect := tree.Bounds[transformed.ID], tree.Bounds[fixed.ID]
	if fixedRect != (Rect{X: transformedRect.X + 35, Y: transformedRect.Y + 25, Width: 35, Height: 50}) {
		t.Fatalf("transformed fixed containing block = host:%#v fixed:%#v", transformedRect, fixedRect)
	}
}

func TestInitialContainingBlockSeparatesAbsoluteAndFixedDuringScroll(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	absolute := document.CreateElement("div", map[string]string{"class": "absolute"})
	fixed := document.CreateElement("div", map[string]string{"class": "fixed"})
	appendNodes(t, document, [2]*dom.Node{document.Root, absolute}, [2]*dom.Node{document.Root, fixed})
	stylesheet, err := css.Parse(strings.NewReader(`
.absolute, .fixed { left:10%; top:20%; width:25%; height:10%; background:#aaa }
.absolute { position:absolute }
.fixed { position:fixed }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithScroll(document, style.Compute(document, stylesheet), 400, 300, 40, 120)
	if got := tree.Bounds[absolute.ID]; got != (Rect{X: 40, Y: 60, Width: 100, Height: 30}) {
		t.Fatalf("initial absolute containing block = %#v", got)
	}
	if got := tree.Bounds[fixed.ID]; got != (Rect{X: 80, Y: 180, Width: 100, Height: 30}) {
		t.Fatalf("scrolled fixed containing block = %#v", got)
	}
}

// Adapted from CSS Positioned Layout over-constraint and stacking assertions.
// Real navigation badges commonly specify both inline insets plus a fixed size.
func TestRealSitePositionedInsetsRespectDirectionAndZOrder(t *testing.T) {
	document := dom.NewDocument()
	host := document.CreateElement("nav", map[string]string{"class": "host"})
	ltr := document.CreateElement("span", map[string]string{"class": "badge ltr"})
	rtl := document.CreateElement("span", map[string]string{"class": "badge rtl"})
	front := document.CreateElement("span", map[string]string{"class": "front"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, host}, [2]*dom.Node{host, ltr}, [2]*dom.Node{host, rtl}, [2]*dom.Node{host, front},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.host { position:relative; width:300px; height:80px; padding:10px }
.badge { position:absolute; left:20px; right:30px; top:10px; width:60px; height:30px; background:#ccc }
.ltr { direction:ltr; z-index:1 }
.rtl { direction:rtl; top:40px; z-index:1 }
.front { position:absolute; inset:0; width:40px; height:40px; z-index:2; background:#333 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, style.Compute(document, stylesheet), 500, 300)
	hostRect := tree.Bounds[host.ID]
	if got := tree.Bounds[ltr.ID].X; got != hostRect.X+20 {
		t.Fatalf("LTR over-constrained inset x = %v, want %v", got, hostRect.X+20)
	}
	if got := tree.Bounds[rtl.ID].X; got != hostRect.X+hostRect.Width-30-60 {
		t.Fatalf("RTL over-constrained inset x = %v, want %v", got, hostRect.X+hostRect.Width-30-60)
	}
	frontRect := tree.Bounds[front.ID]
	if hit, ok := HitTest(tree, frontRect.X+1, frontRect.Y+1); !ok || hit != front.ID {
		t.Fatalf("positioned z-order hit = (%d, %v), want %d", hit, ok, front.ID)
	}
}
