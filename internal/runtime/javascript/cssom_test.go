package javascript

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestCSSOMGeometryAndMediaQueriesUseBrowserSnapshots(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main id="target" style="display:block">content</main>`))
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	var revisions []uint64
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(),
		Media: runtimemodel.MediaEnvironment{ViewportWidth: 800, ViewportHeight: 600, ColorScheme: "light", Hover: true, Pointer: "fine"},
		ReadRender: func(_ context.Context, _ dom.NodeID) (runtimemodel.RenderSnapshot, error) {
			revisions = append(revisions, 17)
			return runtimemodel.RenderSnapshot{
				Revision: 17, Style: map[string]string{"display": "block", "background-color": "rgb(1, 2, 3)", "--token": "ready"},
				Rect:        runtimemodel.DOMRect{X: 10, Y: 20, Width: 240, Height: 80},
				ClientWidth: 236, ClientHeight: 76, ScrollWidth: 300, ScrollHeight: 120,
			}, nil
		},
		ConsoleRecord: func(_, value string) { messages = append(messages, value) },
	}
	source := `
		const target = document.getElementById("target");
		const style = getComputedStyle(target);
		const rect = target.getBoundingClientRect();
		const media = matchMedia("(max-width: 500px), (prefers-reduced-motion: reduce)");
		media.addEventListener("change", function (event) { console.log("change:" + event.matches + ":" + (event.target === media)); });
		console.log([
			target.style instanceof CSSStyleDeclaration, style instanceof CSSStyleDeclaration,
			style.display, style.backgroundColor, style.getPropertyValue("--token"),
			rect.x, rect.top, rect.right, rect.bottom,
			target.clientWidth, target.clientHeight, target.scrollWidth, target.scrollHeight,
			innerWidth, innerHeight, media.matches
		].join("|"));
		try { style.setProperty("display", "none"); } catch (error) { console.log("readonly"); }`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	runtime.UpdateMediaEnvironment(runtimemodel.MediaEnvironment{ViewportWidth: 400, ViewportHeight: 600, ColorScheme: "light", Hover: true, Pointer: "fine", ReducedMotion: true})
	wantMessages := []string{
		"true|true|block|rgb(1, 2, 3)|ready|10|20|250|100|236|76|300|120|800|600|false",
		"readonly", "change:true:true",
	}
	if !reflect.DeepEqual(messages, wantMessages) {
		t.Fatalf("CSSOM messages = %#v, want %#v", messages, wantMessages)
	}
	if len(revisions) != 6 {
		t.Fatalf("render snapshot reads = %v, want 6 reads from revision 17", revisions)
	}
	for _, revision := range revisions {
		if revision != 17 {
			t.Fatalf("render revisions = %v", revisions)
		}
	}
}
