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

func TestBuildAppliesBoxModelAndDisplay(t *testing.T) {
	document := dom.NewDocument()
	visible := document.CreateElement("p", map[string]string{"class": "visible"})
	hidden := document.CreateElement("p", map[string]string{"class": "hidden"})
	link := document.CreateElement("a", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, visible},
		[2]*dom.Node{visible, document.CreateText("Hello ")},
		[2]*dom.Node{visible, link},
		[2]*dom.Node{link, document.CreateText("world")},
		[2]*dom.Node{visible, document.CreateText("!")},
		[2]*dom.Node{document.Root, hidden},
		[2]*dom.Node{hidden, document.CreateText("Secret")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.visible { margin: 10px 30px 14px 20px; padding: 4px 12px 6px 8px; }
.hidden { display: none; }
`))
	if err != nil {
		t.Fatal(err)
	}

	tree := Build(document, style.Compute(document, stylesheet), 800)
	if got, want := len(tree.Boxes), 1; got != want {
		t.Fatalf("box count = %d, want %d", got, want)
	}
	box := tree.Boxes[0]
	if box.Text != "Hello world!" {
		t.Fatalf("text = %q, want inline content on one flow", box.Text)
	}
	if box.X != 60 || box.Y != 46 || box.Width != 666 {
		t.Fatalf("box geometry = (%v, %v, %v), want (60, 46, 666)", box.X, box.Y, box.Width)
	}
}

func TestBuildPreservesInlineRunStyles(t *testing.T) {
	document := dom.NewDocument()
	p := document.CreateElement("p", nil)
	span := document.CreateElement("span", map[string]string{"class": "accent"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, p},
		[2]*dom.Node{p, document.CreateText("Hello ")},
		[2]*dom.Node{p, span},
		[2]*dom.Node{span, document.CreateText("Growse")},
		[2]*dom.Node{p, document.CreateText("!")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`.accent { color: red; font-size: 24px; font-weight: bold; }`))
	if err != nil {
		t.Fatal(err)
	}

	tree := Build(document, style.Compute(document, stylesheet), 800)
	if got, want := len(tree.Boxes), 1; got != want {
		t.Fatalf("line count = %d, want %d", got, want)
	}
	line := tree.Boxes[0]
	if line.Text != "Hello Growse!" || len(line.Runs) != 3 {
		t.Fatalf("line = %#v, want three styled runs", line)
	}
	accent := line.Runs[1]
	if accent.Text != "Growse" || accent.Color != 0xff0000ff || accent.FontSize != 24 || !accent.Bold {
		t.Fatalf("accent run = %#v, want styled Growse", accent)
	}
	if line.Height != 24*1.4 {
		t.Fatalf("line height = %v, want largest run height", line.Height)
	}
}

func TestBuildIncludesGeneratedContentInLayoutAndHitTesting(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, paragraph},
		[2]*dom.Node{paragraph, document.CreateText("middle")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
p::before { content: "before "; }
p::after { content: " after"; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 800)
	if got, want := len(tree.Boxes), 1; got != want {
		t.Fatalf("box count = %d, want %d", got, want)
	}
	box := tree.Boxes[0]
	if got, want := box.Text, "before middle after"; got != want {
		t.Fatalf("generated text = %q, want %q", got, want)
	}
	if got, want := len(box.Runs), 3; got != want {
		t.Fatalf("run count = %d, want %d", got, want)
	}
	if box.Runs[0].Tag != "::before" || box.Runs[2].Tag != "::after" {
		t.Fatalf("generated runs = %#v", box.Runs)
	}
	if got, ok := HitTest(tree, box.X+1, box.Y+1); !ok || got != paragraph.ID {
		t.Fatalf("generated content hit = (%d, %v), want paragraph %d", got, ok, paragraph.ID)
	}
}

func TestBuildCreatesTextInputBox(t *testing.T) {
	document := dom.NewDocument()
	input := document.CreateElement("input", map[string]string{"type": "text", "value": "hello"})
	appendNodes(t, document, [2]*dom.Node{document.Root, input})

	tree := Build(document, style.Compute(document, nil), 800)
	if got, want := len(tree.Boxes), 1; got != want {
		t.Fatalf("box count = %d, want %d", got, want)
	}
	box := tree.Boxes[0]
	if !box.Input || box.NodeID != input.ID || box.Text != "hello" {
		t.Fatalf("input box = %#v, want text input value", box)
	}
	if box.Width != inputWidth || box.Height != inputHeight {
		t.Fatalf("input size = (%v, %v), want (%v, %v)", box.Width, box.Height, inputWidth, inputHeight)
	}
}

func TestBuildIgnoresUnsupportedInputType(t *testing.T) {
	document := dom.NewDocument()
	input := document.CreateElement("input", map[string]string{"type": "checkbox"})
	appendNodes(t, document, [2]*dom.Node{document.Root, input})

	if boxes := Build(document, style.Compute(document, nil), 800).Boxes; len(boxes) != 0 {
		t.Fatalf("boxes = %#v, want unsupported input omitted", boxes)
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
