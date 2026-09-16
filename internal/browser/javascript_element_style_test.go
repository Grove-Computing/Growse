package browser

import (
	"context"
	"strings"
	"testing"

	layoutengine "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	runtimejavascript "github.com/Grove-Computing/Growse/internal/runtime/javascript"
)

func TestJavaScriptElementMutationPublishesStyleAndLayoutRevision(t *testing.T) {
	pageURL := mustParseURL(t, "https://app.example/element-api")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<html><head><style>#target.ready { display: block; }</style></head><body><main id="host"><p id="target">before</p></main><script>
			var target = document.getElementById("target");
			target.classList.add("ready");
			target.style.cssText = "width: 240px; font-size: 28px; color: rgb(255, 0, 0)";
			document.getElementById("host").insertAdjacentHTML("beforeend", '<section id="inserted" style="height: 36px">after</section>');
		</script></body></html>`),
	}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime {
		if engine == runtimemodel.EngineJavaScript {
			return runtimejavascript.New()
		}
		return nil
	})
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	target, targetOK := page.Document.GetElementByID("target")
	inserted, insertedOK := page.Document.GetElementByID("inserted")
	if !targetOK || !insertedOK {
		t.Fatalf("JavaScript HTML mutation targets = target:%v inserted:%v", targetOK, insertedOK)
	}
	computed := page.ComputedStyles[target.ID]
	if page.StyleRevision <= 1 || computed.FontSize != 28 || computed.Color != 0xff0000ff {
		t.Fatalf("style revision/result = %d %#v", page.StyleRevision, computed)
	}
	tree := layoutengine.BuildAtRevision(page.Document, page.ComputedStyles, 800, page.StyleRevision)
	targetBounds, targetLayout := tree.Bounds[target.ID]
	_, insertedLayout := tree.Bounds[inserted.ID]
	if !targetLayout || !insertedLayout || targetBounds.Width != 240 || tree.Revision != page.StyleRevision {
		t.Fatalf("layout revision/result = revision:%d target:%+v targetOK:%v insertedOK:%v", tree.Revision, targetBounds, targetLayout, insertedLayout)
	}
}

func TestJavaScriptCSSOMReadsBrowserStyleAndLayoutRevision(t *testing.T) {
	pageURL := mustParseURL(t, "https://app.example/cssom")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<html><head><style>#target { display:grid; width:240px; height:40px; border:2px solid red; }</style></head><body><main id="target">content</main><script>
			var target = document.getElementById("target");
			var style = getComputedStyle(target);
			var rect = target.getBoundingClientRect();
			target.setAttribute("data-cssom", [style.display, style.width, rect.width, target.clientWidth, target.scrollWidth].join("|"));
		</script></body></html>`),
	}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime {
		if engine == runtimemodel.EngineJavaScript {
			return runtimejavascript.New()
		}
		return nil
	})
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	target, ok := page.Document.GetElementByID("target")
	if !ok {
		t.Fatal("CSSOM target is missing")
	}
	if result := target.Attributes["data-cssom"]; !strings.HasPrefix(result, "grid|240px|244|") {
		t.Fatalf("JavaScript CSSOM result = %q", result)
	}
	snapshot, err := pageRenderSnapshot(context.Background(), page, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != page.StyleRevision || snapshot.Rect.Width != 244 || snapshot.Style["display"] != "grid" {
		t.Fatalf("render snapshot/revision = %#v, page revision %d", snapshot, page.StyleRevision)
	}
}

func TestCSSOMReportsElementScrollExtentAndOverflowClip(t *testing.T) {
	pageURL := mustParseURL(t, "https://app.example/overflow-cssom")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<html><head><style>
			#target { width:100px; height:80px; overflow:auto; }
			#content { width:240px; height:160px; }
			#clipped { overflow:clip; }
		</style></head><body><main id="target"><div id="content"></div></main><aside id="clipped"></aside></body></html>`),
	}}
	browserState := New(loader)
	t.Cleanup(func() { _ = browserState.Close() })
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	target, ok := page.Document.GetElementByID("target")
	if !ok {
		t.Fatal("overflow target is missing")
	}
	snapshot, err := pageRenderSnapshot(context.Background(), page, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ClientWidth != 100 || snapshot.ClientHeight != 80 || snapshot.ScrollWidth != 240 || snapshot.ScrollHeight != 160 {
		t.Fatalf("CSSOM scroll geometry = %#v", snapshot)
	}
	if snapshot.Style["overflow-x"] != "auto" || snapshot.Style["overflow-y"] != "auto" {
		t.Fatalf("CSSOM overflow style = %#v", snapshot.Style)
	}
	clipped, _ := page.Document.GetElementByID("clipped")
	clipSnapshot, err := pageRenderSnapshot(context.Background(), page, clipped.ID)
	if err != nil {
		t.Fatal(err)
	}
	if clipSnapshot.Style["overflow-x"] != "clip" || clipSnapshot.Style["overflow-y"] != "clip" {
		t.Fatalf("CSSOM clip style = %#v", clipSnapshot.Style)
	}
}

func TestJavaScriptMatchMediaChangeFollowsBrowserViewport(t *testing.T) {
	pageURL := mustParseURL(t, "https://app.example/media-change")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<html><body><main id="target"></main><script>
			var media = matchMedia("(max-width: 600px)");
			document.getElementById("target").setAttribute("data-initial", String(media.matches));
			media.addEventListener("change", function(event) {
				document.getElementById("target").setAttribute("data-change", event.matches + ":" + innerWidth);
			});
		</script></body></html>`),
	}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime {
		if engine == runtimemodel.EngineJavaScript {
			return runtimejavascript.New()
		}
		return nil
	})
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	target, _ := page.Document.GetElementByID("target")
	if target.Attributes["data-initial"] != "false" {
		t.Fatalf("initial matchMedia = %q", target.Attributes["data-initial"])
	}
	if !browserState.UpdateViewport(500, 700) {
		t.Fatal("UpdateViewport() = false")
	}
	if target.Attributes["data-change"] != "true:500" {
		t.Fatalf("matchMedia change = %q", target.Attributes["data-change"])
	}
}
