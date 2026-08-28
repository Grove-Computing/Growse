package browser

import (
	"context"
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
