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

func TestDocumentRootsFragmentsAndTemplateContentStayConnectedCorrectly(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<html><head><style id="active">main { color: red }</style></head><body>
		<template id="card"><article id="template-child">card</article><script id="template-script">throw new Error("must not run")</script><style id="template-style">body { color: blue }</style></template>
		<main id="host"></main><script id="boot"></script></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	var message string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		var template = document.getElementById("card");
		var content = template.content;
		var contentChild = content.children[0];
		var imported = document.importNode(content, true);
		var fragment = document.createDocumentFragment();
		var hydrated = document.createElementNS("http://www.w3.org/1999/xhtml", "section");
		hydrated.id = "hydrated";
		fragment.appendChild(hydrated);
		var host = document.getElementById("host");
		host.appendChild(fragment);
		console.log([
			document.documentElement.tagName,
			document.head.tagName,
			document.body.tagName,
			document.currentScript.id,
			document.scripts.length,
			document.styleSheets.length,
			content instanceof DocumentFragment,
			content.isConnected,
			contentChild.isConnected,
			document.getElementById("template-child") === null,
			imported instanceof DocumentFragment,
			imported.children.length,
			fragment.children.length,
			hydrated.isConnected,
			document.getElementById("hydrated") === hydrated,
			template.innerHTML.indexOf("template-child") >= 0
		].join("|"));`
	environment := runtimemodel.Environment{
		Document: document,
		ConsoleRecord: func(_, value string) {
			message = value
		},
	}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{{
		Engine: runtimemodel.EngineJavaScript, Inline: true, DocumentOrder: 0, Source: source,
	}}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "HTML|HEAD|BODY|boot|1|1|true|false|false|true|true|3|0|true|true|true"
	if message != want {
		t.Fatalf("Document fragment result = %q, want %q", message, want)
	}
}
