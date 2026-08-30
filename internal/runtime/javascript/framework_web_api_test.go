package javascript

import (
	"context"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestFrameworkWebAPIContractHasObservableFocusPointerRangeAndGlobals(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<html><body><input id="first"><button id="second">Run</button><p id="text">hello</p></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	var message string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		const first = document.getElementById("first");
		const second = document.getElementById("second");
		const focus = [];
		first.addEventListener("focus", event => focus.push("first:" + (document.activeElement === first) + ":" + (event instanceof FocusEvent)));
		first.addEventListener("blur", event => focus.push("blur:" + (event.relatedTarget === second)));
		second.addEventListener("focus", event => focus.push("second:" + (event.relatedTarget === first)));
		first.focus();
		second.focus();
		second.blur();

		let pointer = "";
		second.addEventListener("pointerdown", event => {
			pointer = [event instanceof PointerEvent, event instanceof MouseEvent, event.pointerId, event.pointerType, event.pressure, event.clientX].join(":");
		});
		second.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true, pointerId: 7, pointerType: "mouse", pressure: 0.5, clientX: 12, isPrimary: true }));

		const text = document.getElementById("text").firstChild;
		const range = document.createRange();
		range.setStart(text, 1);
		range.setEnd(text, 4);
		const clone = range.cloneRange();
		clone.collapse(false);

		console.log([
			document.activeElement === document.body,
			focus.join(","),
			pointer,
			range instanceof Range,
			range.startContainer === text,
			range.commonAncestorContainer === text,
			range.toString(),
			clone.collapsed,
			typeof MutationObserver,
			typeof ResizeObserver,
			typeof IntersectionObserver,
			second.style instanceof CSSStyleDeclaration,
			typeof TextEncoder,
			typeof performance.now
		].join("|"));`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, runtimemodel.Environment{
		Document: document,
		Events:   events.NewDispatcher(),
		ConsoleRecord: func(_, value string) {
			message = value
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "true|first:true:true,blur:true,second:true|true:true:7:mouse:0.5:12|true|true|true|ell|true|function|function|function|true|function|function"
	if message != want {
		t.Fatalf("framework Web API contract = %q, want %q", message, want)
	}
}
