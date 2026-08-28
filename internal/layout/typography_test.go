package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestTypographyReachesMeasurementLineBoxesAndPaintGeometry(t *testing.T) {
	document := dom.NewDocument()
	copy := document.CreateElement("div", map[string]string{"class": "copy"})
	accent := document.CreateElement("span", map[string]string{"class": "accent"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, copy},
		[2]*dom.Node{copy, document.CreateText("hello ")},
		[2]*dom.Node{copy, accent},
		[2]*dom.Node{accent, document.CreateText("world")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.copy { width:220px; font:italic 700 expanded 20px/28px sans-serif; text-align:center; text-transform:uppercase; text-indent:10px; letter-spacing:2px; word-spacing:3px }
.accent { vertical-align:super }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 400)
	if len(tree.Boxes) != 1 {
		t.Fatalf("line boxes = %#v", tree.Boxes)
	}
	line := tree.Boxes[0]
	if line.Text != "HELLO WORLD" || line.X <= pagePadding+10 || len(line.Runs) != 2 {
		t.Fatalf("transformed/aligned line = %#v", line)
	}
	if line.Runs[0].FontStyle != "italic" || line.Runs[0].FontStretch != "expanded" || line.Runs[0].LetterSpacing != 2 || line.Runs[0].WordSpacing != 3 {
		t.Fatalf("measured run typography = %#v", line.Runs[0])
	}
	if line.Runs[1].VerticalOffset <= 0 || line.Height <= 28 {
		t.Fatalf("vertical alignment line metrics = line %#v run %#v", line, line.Runs[1])
	}
}

func TestWordBreakingAndEllipsisRespectComputedPolicy(t *testing.T) {
	document := dom.NewDocument()
	normal := document.CreateElement("div", map[string]string{"class": "normal"})
	anywhere := document.CreateElement("div", map[string]string{"class": "anywhere"})
	ellipsis := document.CreateElement("div", map[string]string{"class": "ellipsis"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, normal}, [2]*dom.Node{normal, document.CreateText("abcdefghij")},
		[2]*dom.Node{document.Root, anywhere}, [2]*dom.Node{anywhere, document.CreateText("abcdefghij")},
		[2]*dom.Node{document.Root, ellipsis}, [2]*dom.Node{ellipsis, document.CreateText("abcdefghij")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.normal,.anywhere,.ellipsis { width:35px }
.anywhere { overflow-wrap:anywhere }
.ellipsis { white-space:nowrap; overflow:hidden; text-overflow:ellipsis }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 300)
	var normalLines, anywhereLines int
	var ellipsisText string
	for _, box := range tree.Boxes {
		switch box.NodeID {
		case normal.ID:
			normalLines++
		case anywhere.ID:
			anywhereLines++
		case ellipsis.ID:
			ellipsisText = box.Text
		}
	}
	if normalLines != 1 || anywhereLines <= 1 {
		t.Fatalf("wrap lines normal=%d anywhere=%d boxes=%#v", normalLines, anywhereLines, tree.Boxes)
	}
	if !strings.HasSuffix(ellipsisText, "…") || ellipsisText == "abcdefghij" {
		t.Fatalf("ellipsis text = %q", ellipsisText)
	}
}
