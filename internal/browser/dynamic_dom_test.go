package browser

import (
	"context"
	"testing"

	"github.com/saku0512/growse/internal/network"
	runtimemodel "github.com/saku0512/growse/internal/runtime"
	"github.com/saku0512/growse/internal/runtime/yaegi"
)

func TestWebGoMutationRecomputesStyles(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/dynamic.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<!doctype html><html><head><style>
.completed { color: #ff0000; }
li.todo { font-size: 24px; }
</style></head><body>
<ul id="list"><li id="existing">Existing</li></ul>
<script type="text/go">package main
import "growse/dom"
func main() {
	dom.GetElementByID("existing").AddClass("completed")
	item := dom.CreateElement("li")
	item.AddClass("todo")
	item.SetText("Dynamic")
	dom.GetElementByID("list").AppendChild(item)
}</script></body></html>`),
	}}
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return yaegi.New() })

	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if !page.RuntimeStarted || page.RuntimeError != "" {
		t.Fatalf("runtime state = started:%v error:%q", page.RuntimeStarted, page.RuntimeError)
	}
	existing, ok := page.Document.GetElementByID("existing")
	if !ok {
		t.Fatal("existing element was not found")
	}
	existingStyle, ok := page.ComputedStyles.For(existing)
	if !ok || existingStyle.Color != 0xff0000ff {
		t.Fatalf("existing style = (%#v, %v), want red", existingStyle, ok)
	}
	dynamic, ok := page.Document.QuerySelector("li.todo")
	if !ok {
		t.Fatal("dynamic element was not found")
	}
	dynamicStyle, ok := page.ComputedStyles.For(dynamic)
	if !ok || dynamicStyle.FontSize != 24 {
		t.Fatalf("dynamic style = (%#v, %v), want 24px", dynamicStyle, ok)
	}
}
