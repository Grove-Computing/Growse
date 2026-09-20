package paint

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestTailwindV4ThemeUtilitiesReachStyleLayoutAndPaint(t *testing.T) {
	document := dom.NewDocument()
	card := document.CreateElement("section", map[string]string{
		"class": "grid grid-cols-2 max-w-2xl gap-4 rounded-lg bg-white p-5",
	})
	first := document.CreateElement("article", map[string]string{"class": "item"})
	second := document.CreateElement("article", map[string]string{"class": "item"})
	if err := document.AppendChild(document.Root, card); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(card, first); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(card, second); err != nil {
		t.Fatal(err)
	}

	stylesheet, err := css.Parse(strings.NewReader(`
:root,:host {
  --spacing:.25rem;
  --container-2xl:42rem;
  --radius-lg:.5rem;
  --color-white:#fff;
}
.grid { display:grid }
.grid-cols-2 { grid-template-columns:repeat(2,minmax(0,1fr)) }
.max-w-2xl { max-width:var(--container-2xl) }
.gap-4 { gap:calc(var(--spacing)*4) }
.rounded-lg { border-radius:var(--radius-lg) }
.bg-white { background-color:var(--color-white) }
.p-5 { padding:calc(var(--spacing)*5) }
.item { height:2rem }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.Compute(document, stylesheet)
	cardStyle, ok := computed.For(card)
	if !ok {
		t.Fatal("card has no computed style")
	}
	if cardStyle.Display != style.DisplayGrid || len(cardStyle.GridTemplateColumns) != 2 {
		t.Fatalf("grid utilities = display:%v columns:%#v", cardStyle.Display, cardStyle.GridTemplateColumns)
	}
	if cardStyle.MaxWidth.Kind != style.SizeLength || cardStyle.MaxWidth.Value.Pixels != 672 {
		t.Fatalf("max-width utility = %#v", cardStyle.MaxWidth)
	}
	if cardStyle.RowGap.Pixels != 16 || cardStyle.ColumnGap.Pixels != 16 || cardStyle.Padding.Top != 20 || cardStyle.Padding.Left != 20 {
		t.Fatalf("spacing utilities = gap:%#v/%#v padding:%#v", cardStyle.RowGap, cardStyle.ColumnGap, cardStyle.Padding)
	}
	if cardStyle.BorderRadius.TopLeft.X.Pixels != 8 || cardStyle.BackgroundColor != 0xffffffff {
		t.Fatalf("visual utilities = radius:%#v background:%08x", cardStyle.BorderRadius, cardStyle.BackgroundColor)
	}

	tree := layout.Build(document, computed, 800)
	var cardDecoration *layout.Decoration
	for index := range tree.Decorations {
		if tree.Decorations[index].NodeID == card.ID {
			cardDecoration = &tree.Decorations[index]
			break
		}
	}
	if cardDecoration == nil || cardDecoration.Width <= 0 || cardDecoration.Width > 712 || cardDecoration.Radius.TopLeft.X != 8 {
		t.Fatalf("Tailwind card layout = %#v", cardDecoration)
	}

	var cardPaint *DrawBox
	for _, command := range Build(tree).Commands {
		if box, ok := command.(DrawBox); ok && box.NodeID == card.ID {
			cardPaint = &box
			break
		}
	}
	if cardPaint == nil || cardPaint.Color != 0xffffffff || cardPaint.Radius.TopLeft.X != 8 {
		t.Fatalf("Tailwind card paint = %#v", cardPaint)
	}
}

func TestTailwindV4RegisteredShadowVariablesReachPaint(t *testing.T) {
	document := dom.NewDocument()
	card := document.CreateElement("article", map[string]string{"class": "card"})
	if err := document.AppendChild(document.Root, card); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
*,:after,:before,::backdrop { box-sizing:border-box; border:0 solid; margin:0; padding:0 }
@property --tw-shadow { syntax:"*"; inherits:false; initial-value:0 0 #0000 }
@property --tw-inset-shadow { syntax:"*"; inherits:false; initial-value:0 0 #0000 }
@property --tw-ring-offset-shadow { syntax:"*"; inherits:false; initial-value:0 0 #0000 }
@property --tw-ring-shadow { syntax:"*"; inherits:false; initial-value:0 0 #0000 }
.card {
  width:240px; height:120px; padding:24px; border-width:1px; border-color:#f3f4f6; border-radius:8px; background:#fff;
  --tw-shadow:0 1px 3px 0 #0000001a,0 1px 2px -1px #0000001a;
  box-shadow:var(--tw-inset-shadow),var(--tw-ring-offset-shadow),var(--tw-ring-shadow),var(--tw-shadow)
}
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.Compute(document, stylesheet)
	cardStyle, _ := computed.For(card)
	if cardStyle.BoxSizing != style.BoxSizingBorderBox || cardStyle.Border.Top.Style != style.BorderSolid || len(cardStyle.BoxShadows) != 5 {
		t.Fatalf("registered Tailwind shadows = %#v, want transparent reset layers plus two visible layers", cardStyle.BoxShadows)
	}
	tree := layout.Build(document, computed, 320)
	var command *DrawBox
	for _, candidate := range Build(tree).Commands {
		if box, ok := candidate.(DrawBox); ok && box.NodeID == card.ID {
			command = &box
			break
		}
	}
	if command == nil || command.Width != 240 || command.Radius.TopLeft.X != 8 || command.Border.Top.Width != 1 || command.Border.Top.Style != style.BorderSolid || len(command.BoxShadows) != 5 {
		t.Fatalf("Tailwind card boundary paint = %#v", command)
	}
}
