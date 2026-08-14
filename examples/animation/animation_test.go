package animationexample

import (
	"os"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestAnimationShowcaseUsesTransitionsAndKeyframes(t *testing.T) {
	htmlSource, err := os.Open("index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer htmlSource.Close()
	document, err := htmlparser.Parse(htmlSource)
	if err != nil {
		t.Fatal(err)
	}
	cssSource, err := os.ReadFile("style.css")
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(string(cssSource)))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.Compute(document, stylesheet)
	card, ok := document.QuerySelector(".card")
	if !ok {
		t.Fatal("showcase card is missing")
	}
	cardStyle, _ := computed.For(card)
	if len(cardStyle.Transitions) != 3 {
		t.Fatalf("card transitions = %d, want 3", len(cardStyle.Transitions))
	}
	orb, ok := document.QuerySelector(".cyan")
	if !ok {
		t.Fatal("animated orb is missing")
	}
	orbStyle, _ := computed.For(orb)
	if len(stylesheet.Keyframes) != 2 || len(orbStyle.Animations) != 1 || orbStyle.Animations[0].Name != "drift" {
		t.Fatalf("showcase keyframes/animation = %d / %#v", len(stylesheet.Keyframes), orbStyle.Animations)
	}
}
