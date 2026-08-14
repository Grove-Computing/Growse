package css3core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestCSS3CoreDemoReachesStyleLayoutAndPaint(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir(".")))
	defer server.Close()

	browserState := browser.New(network.NewClient())
	defer browserState.Close()
	page, err := browserState.Navigate(context.Background(), server.URL+"/index.html")
	if err != nil {
		t.Fatal(err)
	}
	hero := elementByClass(t, page, "hero")
	card := elementByClass(t, page, "card")
	muted := elementByClass(t, page, "muted")
	overflow := elementByClass(t, page, "overflow-demo")

	heroStyle, _ := page.ComputedStyles.For(hero)
	if heroStyle.BackgroundImage.Kind != style.BackgroundImageLinearGradient || len(heroStyle.BackgroundImage.GradientStops) != 3 || heroStyle.BorderRadius.TopLeft.X.Pixels != 18 {
		t.Fatalf("hero style = %#v", heroStyle)
	}
	cardStyle, _ := page.ComputedStyles.For(card)
	if cardStyle.Display != style.DisplayInlineBlock || cardStyle.BackgroundColor != 0xf8fafcff || cardStyle.Padding.Top != 16 {
		t.Fatalf("card style = %#v", cardStyle)
	}
	mutedStyle, _ := page.ComputedStyles.For(muted)
	if mutedStyle.Opacity != .72 {
		t.Fatalf("muted opacity = %v", mutedStyle.Opacity)
	}
	overflowStyle, _ := page.ComputedStyles.For(overflow)
	if overflowStyle.OverflowX != style.OverflowHidden || overflowStyle.WhiteSpace != style.WhiteSpaceNowrap {
		t.Fatalf("overflow style = %#v", overflowStyle)
	}

	tree := layoutmodel.BuildWithViewport(page.Document, page.ComputedStyles, 900, 640)
	list := paintmodel.Build(tree)
	var hasGradient, hasRoundedBorder, hasClip bool
	for _, command := range list.Commands {
		switch command := command.(type) {
		case paintmodel.DrawBox:
			hasGradient = hasGradient || command.Image.Kind == style.BackgroundImageLinearGradient
			hasRoundedBorder = hasRoundedBorder || command.Radius.TopLeft.X > 0 && command.Border.Top.Width > 0
		case paintmodel.DrawText:
			hasClip = hasClip || command.Clip != nil
		}
	}
	if !hasGradient || !hasRoundedBorder || !hasClip {
		t.Fatalf("display list features = gradient:%v rounded-border:%v clip:%v", hasGradient, hasRoundedBorder, hasClip)
	}

	for _, path := range []string{"index.html", "style.css"} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("demo file %s is unavailable or empty: %v", path, err)
		}
	}
}

func elementByClass(t *testing.T, page *browser.Page, className string) *dom.Node {
	t.Helper()
	var found *dom.Node
	var walk func(*dom.Node) bool
	walk = func(node *dom.Node) bool {
		if node == nil {
			return true
		}
		if node.Type == dom.NodeElement && hasClass(node, className) {
			found = node
			return false
		}
		for _, child := range node.Children {
			if !walk(child) {
				return false
			}
		}
		return true
	}
	walk(page.Document.Root)
	if found == nil {
		t.Fatalf(".%s was not found", className)
	}
	return found
}

func hasClass(node *dom.Node, target string) bool {
	value, _ := node.Attribute("class")
	for _, className := range strings.Fields(value) {
		if className == target {
			return true
		}
	}
	return false
}
