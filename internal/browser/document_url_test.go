package browser

import (
	"context"
	"slices"
	"strings"
	"testing"

	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestDocumentBaseURLAppliesToPageResourcesAndActions(t *testing.T) {
	documentURL := mustParseURL(t, "https://base.example/root/index.html")
	baseURL := mustParseURL(t, "https://base.example/assets/")
	styleURL := mustParseURL(t, "https://base.example/assets/theme.css")
	classicURL := mustParseURL(t, "https://base.example/assets/classic.js")
	moduleURL := mustParseURL(t, "https://base.example/assets/module.js")
	frameURL := mustParseURL(t, "https://base.example/assets/frame.html")
	imageURL := "https://base.example/assets/images/card.png"
	submitURL := mustParseURL(t, "https://base.example/assets/submit?q=value")
	loader := &routeLoader{responses: map[string]*network.Response{
		documentURL.String(): {
			URL: documentURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<base href="http://[invalid"><base href="/assets/"><base href="/ignored/">
				<link rel="stylesheet" href="theme.css">
				<a id="link" href="next.html">next</a>
				<div id="card" class="card">card</div>
				<form id="form" action="submit"><input name="q" value="value"><button id="submit">send</button></form>
				<iframe id="frame" src="frame.html"></iframe>
				<script type="importmap">{"imports":{"fixture":"lib.js"}}</script>
				<script src="classic.js"></script><script type="module" src="module.js"></script>`),
		},
		styleURL.String():   {URL: styleURL, StatusCode: 200, ContentType: "text/css", Body: []byte(`.card { background-image: url("images/card.png") }`)},
		classicURL.String(): {URL: classicURL, StatusCode: 200, ContentType: "text/javascript", Body: []byte(`globalThis.classicLoaded = true`)},
		moduleURL.String():  {URL: moduleURL, StatusCode: 200, ContentType: "text/javascript", Body: []byte(`export const loaded = true`)},
		frameURL.String():   {URL: frameURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<p>frame</p>`)},
		submitURL.String():  {URL: submitURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<p>submitted</p>`)},
	}}
	var loadedRuntime *runtimeStub
	browserState := NewWithEngineFactory(loader, func(runtimemodel.Engine) runtimemodel.Runtime {
		loadedRuntime = &runtimeStub{}
		return loadedRuntime
	})
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), documentURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if page.URL.String() != documentURL.String() || page.BaseURL.String() != baseURL.String() {
		t.Fatalf("document/base URL = %s / %s", page.URL, page.BaseURL)
	}
	link, _ := page.Document.GetElementByID("link")
	if target, ok := page.LinkURL(link.ID); !ok || target.String() != "https://base.example/assets/next.html" {
		t.Fatalf("base-resolved anchor = %v, %t", target, ok)
	}
	card, _ := page.Document.GetElementByID("card")
	computed, ok := page.ComputedStyles.For(card)
	if !ok || computed.BackgroundImage.URL != imageURL || !slices.Contains(loader.requested, imageURL) {
		t.Fatalf("base-resolved CSS resource = %#v, requested=%v", computed.BackgroundImage, loader.requested)
	}
	if len(page.Scripts) != 2 || page.Scripts[0].SourceURL.String() != classicURL.String() || page.Scripts[1].SourceURL.String() != moduleURL.String() {
		t.Fatalf("base-resolved scripts = %#v", page.Scripts)
	}
	if page.ImportMap["fixture"] != "https://base.example/assets/lib.js" {
		t.Fatalf("base-resolved import map = %#v", page.ImportMap)
	}
	frame, _ := page.Document.GetElementByID("frame")
	loadedFrame, ok := page.FrameByElement(frame.ID)
	if !ok || loadedFrame.URL.String() != frameURL.String() {
		t.Fatalf("base-resolved iframe = %#v", loadedFrame)
	}
	if loadedRuntime == nil || loadedRuntime.environment.BaseURL.String() != documentURL.String() || loadedRuntime.environment.ResourceBaseURL.String() != baseURL.String() {
		t.Fatalf("runtime document/resource bases = %#v", loadedRuntime)
	}
	form, _ := page.Document.GetElementByID("form")
	submitter, _ := page.Document.GetElementByID("submit")
	submitted, err := browserState.SubmitGET(context.Background(), form.ID, submitter.ID)
	if err != nil || submitted.URL.String() != submitURL.String() {
		t.Fatalf("base-resolved form = %v, %v", submitted, err)
	}
}

func TestDocumentBaseURLKeepsFirstValidValueAndRemovesCredentials(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<base href="http://[invalid"><base href="https://user:secret@cdn.example/assets/"><base href="/ignored/">`))
	if err != nil {
		t.Fatal(err)
	}
	base := documentBaseURL(document, mustParseURL(t, "https://page.example/root/index.html"))
	if got, want := base.String(), "https://cdn.example/assets/"; got != want || base.User != nil {
		t.Fatalf("document base = %q user=%v, want %q without credentials", got, base.User, want)
	}
	fallback := documentBaseURL(nil, mustParseURL(t, "https://user:secret@page.example/root/index.html"))
	if got, want := fallback.String(), "https://page.example/root/index.html"; got != want || fallback.User != nil {
		t.Fatalf("fallback base = %q user=%v, want %q without credentials", got, fallback.User, want)
	}
}
