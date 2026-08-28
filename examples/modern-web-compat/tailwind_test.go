package main

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestTailwindFixtureAppliesModernUtilitiesToGeometryAndPaint(t *testing.T) {
	server := httptest.NewServer(modernWebCompatibilityHandler())
	defer server.Close()
	engine := browser.New(network.NewClientWithLimits(server.Client(), 4<<20))
	defer engine.Close()

	page, err := engine.Navigate(context.Background(), server.URL+"/tailwind/")
	if err != nil {
		t.Fatal(err)
	}
	if !engine.UpdateViewport(1024, 720) {
		t.Fatal("wide viewport was not applied")
	}
	page = engine.Page()
	root := fixtureNode(t, page, "tailwind-root")
	grid := fixtureNode(t, page, "tailwind-grid")
	card := fixtureNode(t, page, "tailwind-card")
	button := fixtureNode(t, page, "tailwind-button")

	rootStyle, _ := page.ComputedStyles.For(root)
	gridStyle, _ := page.ComputedStyles.For(grid)
	cardStyle, _ := page.ComputedStyles.For(card)
	buttonStyle, _ := page.ComputedStyles.For(button)
	if rootStyle.ContainerName != "fixture" || rootStyle.ContainerType != style.ContainerTypeInlineSize {
		t.Fatalf("Tailwind container style = %#v", rootStyle)
	}
	if gridStyle.Display != style.DisplayGrid || len(gridStyle.GridTemplateColumns) != 2 {
		t.Fatalf("responsive utility style = %#v", gridStyle)
	}
	if cardStyle.Width.Kind != style.SizeLength || cardStyle.Width.Value.Pixels != 200 || cardStyle.Padding.Top != 12 {
		t.Fatalf("container/custom-property utility style = %#v", cardStyle)
	}
	if cardStyle.BackgroundColor != 0x111827ff || cardStyle.Color != 0xffffffff {
		t.Fatalf("dark variant paint style = color:%#08x background:%#08x", cardStyle.Color, cardStyle.BackgroundColor)
	}
	if buttonStyle.BoxSizing != style.BoxSizingBorderBox || buttonStyle.Margin != (style.Edges{}) {
		t.Fatalf("preflight style = %#v", buttonStyle)
	}

	tree := layoutmodel.BuildWithViewport(page.Document, page.ComputedStyles, 1024, 720)
	bounds, ok := tree.Bounds[card.ID]
	if !ok || bounds.Width != 200 {
		t.Fatalf("Tailwind card geometry = %#v, found=%t", bounds, ok)
	}
	list := paintmodel.Build(tree)
	paintedDarkCard := false
	for _, command := range list.Commands {
		box, ok := command.(paintmodel.DrawBox)
		if ok && box.NodeID == card.ID && box.Color == 0x111827ff {
			paintedDarkCard = true
		}
	}
	if !paintedDarkCard {
		t.Fatal("dark variant did not reach the paint display list")
	}

	if !engine.UpdateViewport(640, 720) {
		t.Fatal("narrow viewport was not applied")
	}
	narrowGrid, _ := engine.Page().ComputedStyles.For(grid)
	if narrowGrid.Display != style.DisplayBlock {
		t.Fatalf("narrow responsive utility display = %v", narrowGrid.Display)
	}
}
