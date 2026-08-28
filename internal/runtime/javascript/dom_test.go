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
		if (!removed.setAttribute("data-state", "after") || removed.getAttribute("data-state") !== "after") { throw new Error("disconnected identity was lost"); }
		if (app.appendChild({}) !== null) { throw new Error("foreign object appended"); }`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}

func TestJavaScriptNodeRelationshipsMutationsFragmentsAndCloneKeepIdentity(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<html><head></head><body><main id="host"><p id="a">A</p><p id="b">B</p></main></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	var message string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		var host = document.getElementById("host");
		var a = document.getElementById("a");
		var b = document.getElementById("b");
		var clone = a.cloneNode(true);
		clone.id = "clone";
		var fragment = document.createDocumentFragment();
		var x = document.createElement("span"); x.id = "x";
		var y = document.createElement("span"); y.id = "y";
		fragment.append(x, y);
		var inserted = host.insertBefore(fragment, b);
		var replaced = host.replaceChild(clone, a);
		var cycleRejected = x.appendChild(host) === null;
		b.before("before-b");
		b.after("after-b");
		b.remove();
		b.setAttribute("data-detached", "yes");
		host.appendChild(b);
		var replacement = document.createElement("strong"); replacement.id = "replacement";
		x.replaceWith(replacement, "tail");
		console.log([
			document.nodeType,
			document.nodeName,
			document.documentElement.parentNode === document,
			host.nodeType,
			host.nodeName,
			host.parentNode === document.body,
			host.firstChild === clone,
			host.lastChild === b,
			clone.nextSibling === replacement,
			y.previousSibling.nodeType === 3,
			host.childNodes.item(0) === clone,
			inserted === fragment,
			fragment.childNodes.length,
			replaced === a,
			!a.isConnected,
			clone !== a && clone.firstChild !== a.firstChild,
			cycleRejected,
			host.contains(replacement),
			!replacement.contains(host),
			document.getElementById("b") === b,
			b.getAttribute("data-detached"),
			host.childNodes.length
		].join("|"));`
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher(), ConsoleRecord: func(_, value string) { message = value }}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "9|#document|true|1|MAIN|true|true|true|true|true|true|true|0|true|true|true|true|true|true|true|yes|7"
	if message != want {
		t.Fatalf("Node mutation result = %q, want %q", message, want)
	}
}

func TestJavaScriptDOMCollectionsTreeMetadataInnerHTMLAndClassList(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main id="app"><p class="card featured">one</p><p class="card">two</p></main>`))
	if err != nil {
		t.Fatal(err)
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		var app = document.getElementById("app");
		if (document.querySelectorAll(".card").length !== 2) throw new Error("querySelectorAll");
		if (document.getElementsByClassName("card featured").length !== 1) throw new Error("class collection");
		if (document.getElementsByTagName("p").length !== 2) throw new Error("tag collection");
		if (app.id !== "app" || app.tagName !== "MAIN" || app.children.length !== 2) throw new Error("metadata");
		var text = document.createTextNode("prefix");
		var section = document.createElement("section");
		section.id = "section";
		section.className = "panel";
		app.prepend(text);
		app.append(section, "suffix");
		if (section.parentElement !== app || !section.classList.contains("panel")) throw new Error("parent/class contains");
		if (section.classList.toggle("panel") !== false || section.classList.toggle("active", true) !== true) throw new Error("class toggle");
		if (app.removeChild(section) !== section || section.parentElement !== null) throw new Error("removeChild");
		app.replaceChildren(section);
		app.innerHTML = '<article id="article"><strong class="label">safe &amp; sound</strong><script>globalThis.mustNotRun = true</script></article>';
		if (app.children.length !== 1 || app.innerHTML.indexOf("safe &amp; sound") < 0 || globalThis.mustNotRun === true) throw new Error("innerHTML");`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, runtimemodel.Environment{Document: document, Events: events.NewDispatcher()}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	article, ok := document.GetElementByID("article")
	if !ok || article.Parent == nil || article.Parent.TagName != "main" || document.OwnedNodeCount() <= document.NodeCount() {
		t.Fatalf("JavaScript DOM result = article:%#v connected:%d owned:%d", article, document.NodeCount(), document.OwnedNodeCount())
	}
}
