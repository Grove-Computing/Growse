package browser

import (
	"context"
	"testing"

	layoutengine "github.com/saku0512/growse/internal/layout"
	"github.com/saku0512/growse/internal/network"
	paintmodel "github.com/saku0512/growse/internal/paint"
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
	existing := dom.GetElementByID("existing")
	existing.AddClass("completed")
	item := dom.CreateElement("li")
	item.AddClass("todo")
	item.SetText("Dynamic")
	dom.GetElementByID("list").AppendChild(item)
	existing.OnClick(func() { item.Remove() })
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

	tree := layoutengine.Build(page.Document, page.ComputedStyles, 800)
	displayList := paintmodel.Build(tree)
	if !displayListContainsText(displayList, "Dynamic") {
		t.Fatal("dynamic element is missing from the initial display list")
	}
	if !browser.DispatchClick(existing.ID, 0, 0) {
		t.Fatal("remove click was not handled")
	}
	if _, ok := page.Document.QuerySelector("li.todo"); ok {
		t.Fatal("dynamic element remains after WebGo removal")
	}
	updatedTree := layoutengine.Build(page.Document, page.ComputedStyles, 800)
	updatedDisplayList := paintmodel.Build(updatedTree)
	if displayListContainsText(updatedDisplayList, "Dynamic") {
		t.Fatal("removed element remains in the updated display list")
	}
}

func displayListContainsText(list *paintmodel.DisplayList, text string) bool {
	if list == nil {
		return false
	}
	for _, command := range list.Commands {
		drawText, ok := command.(paintmodel.DrawText)
		if ok && drawText.Text == text {
			return true
		}
	}
	return false
}
