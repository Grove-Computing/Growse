package javascript

import (
	"context"
	"strings"
	"testing"

	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestDOMInterfacesKeepStableWrappersAndPrototypeChains(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main><button id="action">Run</button></main>`))
	if err != nil {
		t.Fatal(err)
	}
	var message string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		var action = document.getElementById("action");
		var same = document.querySelector("#action");
		var script = document.createElement("script");
		var link = document.createElement("link");
		var image = document.createElement("img");
		var template = document.createElement("template");
		var text = document.createTextNode("text");
		console.log([
			document instanceof Document,
			document instanceof Node,
			document instanceof EventTarget,
			action === same,
			action instanceof HTMLElement,
			action instanceof Element,
			action instanceof Node,
			action instanceof EventTarget,
			action.constructor === HTMLElement,
			Element.prototype.getAttribute.call(action, "id"),
			script instanceof HTMLScriptElement,
			link instanceof HTMLLinkElement,
			image instanceof HTMLImageElement,
			template instanceof HTMLTemplateElement,
			text instanceof Node,
			!(text instanceof Element),
			Object.prototype.toString.call(image)
		].join("|"));`
	environment := runtimemodel.Environment{
		Document: document,
		ConsoleRecord: func(_, value string) {
			message = value
		},
	}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "true|true|true|true|true|true|true|true|true|action|true|true|true|true|true|true|[object HTMLImageElement]"
	if message != want {
		t.Fatalf("DOM interface result = %q, want %q", message, want)
	}
}
