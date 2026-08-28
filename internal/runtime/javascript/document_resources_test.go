package javascript

import (
	"context"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestDocumentResourceCollectionsCurrentScriptAndLifecycleStayConsistent(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main id="host"></main>
		<script id="initial"></script><script id="module" type="module"></script>
		<style id="initial-style">main { color: red; }</style>`))
	if err != nil {
		t.Fatal(err)
	}
	var records [][2]string
	refreshes := 0
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(), BaseURL: moduleTestURL(t, "https://app.example/page"),
		RefreshStyles: func(context.Context) error { refreshes++; return nil },
		ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) },
	}
	classic := runtimemodel.Script{
		Engine: runtimemodel.EngineJavaScript, Kind: runtimemodel.ScriptClassic, DocumentOrder: 0,
		SourceURL: environment.BaseURL,
		Source: `
			console.log("initial:" + document.currentScript.id + ":" + document.scripts.length + ":" + document.scripts.item(0).id + ":" + document.styleSheets.length);
			document.addEventListener("DOMContentLoaded", function () { console.log("dom:" + (document.currentScript === null)); });
			addEventListener("load", function () { console.log("window:" + (document.currentScript === null)); });
			var dynamic = document.createElement("script");
			dynamic.id = "dynamic";
			dynamic.textContent = "console.log('dynamic:' + document.currentScript.id + ':' + document.scripts.length);";
			document.getElementById("host").appendChild(dynamic);
			console.log("restored:" + document.currentScript.id);
			var style = document.createElement("style");
			style.textContent = "main { font-size: 20px; }";
			document.getElementById("host").appendChild(style);
			console.log("styles:" + document.styleSheets.length + ":" + (document.styleSheets.item(0).ownerNode === style));`,
	}
	module := runtimemodel.Script{
		Engine: runtimemodel.EngineJavaScript, Kind: runtimemodel.ScriptModule, Inline: true,
		DocumentOrder: 1, SourceURL: environment.BaseURL, Schedule: runtimemodel.ScriptDefer,
		Source: `console.log("module:" + (document.currentScript === null));`,
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	if err := runtime.Load(context.Background(), []runtimemodel.Script{classic, module}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := [][2]string{
		{"log", "initial:initial:2:initial:1"},
		{"log", "dynamic:dynamic:3"},
		{"log", "restored:initial"},
		{"log", "styles:2:true"},
		{"log", "module:true"},
		{"log", "dom:true"},
		{"log", "window:true"},
	}
	if !equalRecords(records, want) {
		t.Fatalf("resource lifecycle records = %v, want %v", records, want)
	}
	if refreshes != 1 {
		t.Fatalf("style refreshes = %d, want 1", refreshes)
	}
}
