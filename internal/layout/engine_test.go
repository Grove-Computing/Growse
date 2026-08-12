package layout

import (
	"strings"
	"testing"

	"github.com/saku0512/growse/internal/css"
	"github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/style"
)

func TestBuildCreatesVisibleVerticalBoxes(t *testing.T) {
	document := dom.NewDocument()
	html := document.CreateElement("html", nil)
	head := document.CreateElement("head", nil)
	title := document.CreateElement("title", nil)
	body := document.CreateElement("body", nil)
	h1 := document.CreateElement("h1", nil)
	p := document.CreateElement("p", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, html},
		[2]*dom.Node{html, head},
		[2]*dom.Node{head, title},
		[2]*dom.Node{title, document.CreateText("Hidden title")},
		[2]*dom.Node{html, body},
		[2]*dom.Node{body, h1},
		[2]*dom.Node{h1, document.CreateText("Hello")},
		[2]*dom.Node{body, p},
		[2]*dom.Node{p, document.CreateText("World")},
	)

	tree := Build(document, nil, 800)
	if got, want := len(tree.Boxes), 2; got != want {
		t.Fatalf("box count = %d, want %d", got, want)
	}
	if tree.Boxes[0].Text != "Hello" || tree.Boxes[0].Tag != "h1" || !tree.Boxes[0].Bold {
		t.Fatalf("first box = %#v, want bold h1 Hello", tree.Boxes[0])
	}
	if tree.Boxes[1].Text != "World" || tree.Boxes[1].Y <= tree.Boxes[0].Y {
		t.Fatalf("second box = %#v, want World below first box", tree.Boxes[1])
	}
}

func TestBuildWrapsLongText(t *testing.T) {
	document := dom.NewDocument()
	p := document.CreateElement("p", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, p},
		[2]*dom.Node{p, document.CreateText("one two three four five six seven eight nine ten")},
	)

	tree := Build(document, nil, 160)
	if len(tree.Boxes) < 2 {
		t.Fatalf("box count = %d, want wrapped text", len(tree.Boxes))
	}
}

func TestBuildUsesComputedTextStyle(t *testing.T) {
	document := dom.NewDocument()
	p := document.CreateElement("p", map[string]string{"class": "notice"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, p},
		[2]*dom.Node{p, document.CreateText("Styled")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`.notice { color: #abcdef; font-size: 24px; font-weight: bold }`))
	if err != nil {
		t.Fatal(err)
	}

	tree := Build(document, style.Compute(document, stylesheet), 800)
	if len(tree.Boxes) != 1 {
		t.Fatalf("box count = %d, want 1", len(tree.Boxes))
	}
	box := tree.Boxes[0]
	if box.Color != 0xabcdefff || box.FontSize != 24 || !box.Bold {
		t.Fatalf("box style = %#v, want CSS color, size and weight", box)
	}
}

func appendNodes(t *testing.T, document *dom.Document, edges ...[2]*dom.Node) {
	t.Helper()
	for _, edge := range edges {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
}
