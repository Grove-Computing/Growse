package javascript

import (
	"context"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestDocumentAndElementMutationsUseGrowseDOM(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`
		<main id="app"><input id="name" value="before"><p class="old">remove me</p></main>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	mutations := 0
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		Document: document,
		Events:   events.NewDispatcher(),
		OnMutation: func() {
			mutations++
		},
	}
	source := `
		var app = document.querySelector("#app");
		var name = document.getElementById("name");
		var old = document.querySelector(".old");
		if (app !== document.getElementById("app")) { throw new Error("element identity changed"); }
		if (app.ID !== undefined) { throw new Error("internal Node ID leaked"); }
		var created = document.createElement("section");
		created.setAttribute("id", "created");
		created.classList.add("active");
		created.classList.add("selected");
		created.classList.remove("active");
		created.textContent = "created by JavaScript";
		if (app.appendChild(created) !== created) { throw new Error("append failed"); }
		name.value = "after";
		old.remove();`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	created, ok := document.GetElementByID("created")
	if !ok {
		t.Fatal("created element was not attached to the Growse Document")
	}
	if got, want := created.TagName, "section"; got != want {
		t.Fatalf("created tag = %q, want %q", got, want)
	}
	if got, want := created.TextContent(), "created by JavaScript"; got != want {
		t.Fatalf("created text = %q, want %q", got, want)
	}
	if got, _ := created.Attribute("class"); got != "selected" {
		t.Fatalf("created class = %q, want selected", got)
	}
	name, _ := document.GetElementByID("name")
	if got, want := name.ControlValue, "after"; got != want {
		t.Fatalf("input value = %q, want %q", got, want)
	}
	if _, ok := document.QuerySelector(".old"); ok {
		t.Fatal("removed element remains connected")
	}
	if mutations < 7 {
		t.Fatalf("mutation callbacks = %d, want at least 7", mutations)
	}
}

func TestDOMRejectsInvalidAndDisconnectedOperations(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main id="app"><p id="removed">text</p></main>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher()}
	source := `
		if (document.querySelector("main > p") !== null) { throw new Error("unsupported selector accepted"); }
		if (document.createElement("bad tag") !== null) { throw new Error("invalid tag accepted"); }
		var app = document.getElementById("app");
		if (app.setAttribute("bad name", "value")) { throw new Error("invalid attribute accepted"); }
		var removed = document.getElementById("removed");
		removed.remove();
		if (removed.setAttribute("data-state", "after")) { throw new Error("disconnected mutation accepted"); }
		if (app.appendChild({}) !== null) { throw new Error("foreign object appended"); }`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}
