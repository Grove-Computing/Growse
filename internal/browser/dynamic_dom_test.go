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

func TestHoverRebuildsLayoutAndDisplayList(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/hover.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<!doctype html><html><head><style>
button { color: black; font-size: 16px; }
button:hover { color: red; font-size: 24px; padding: 8px; }
</style></head><body><button id="save">Save</button></body></html>`),
	}}
	browserState := New(loader)
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	button, ok := page.Document.GetElementByID("save")
	if !ok {
		t.Fatal("save button was not found")
	}

	normalTree := layoutengine.Build(page.Document, page.ComputedStyles, 800)
	if !browserState.UpdateHover(button.ID, 12, 34) {
		t.Fatal("UpdateHover() = false, want true")
	}
	hoverTree := layoutengine.Build(page.Document, page.ComputedStyles, 800)
	hoverList := paintmodel.Build(hoverTree)
	if len(normalTree.Boxes) == 0 || len(hoverTree.Boxes) == 0 || len(hoverList.Commands) == 0 {
		t.Fatal("hover layout or display list is empty")
	}
	if normalTree.Boxes[0].Height == hoverTree.Boxes[0].Height {
		t.Fatalf("layout height stayed %v after hover font and padding change", hoverTree.Boxes[0].Height)
	}
	draw, ok := hoverList.Commands[0].(paintmodel.DrawText)
	if !ok || len(draw.Runs) != 1 || draw.Runs[0].Color != 0xff0000ff || draw.Runs[0].FontSize != 24 {
		t.Fatalf("hover display command = %#v, want red 24px text", hoverList.Commands[0])
	}
	if !browserState.ClearHover() {
		t.Fatal("ClearHover() = false, want true")
	}
	restoredList := paintmodel.Build(layoutengine.Build(page.Document, page.ComputedStyles, 800))
	restored, ok := restoredList.Commands[0].(paintmodel.DrawText)
	if !ok || len(restored.Runs) != 1 || restored.Runs[0].Color == 0xff0000ff || restored.Runs[0].FontSize != 16 {
		t.Fatalf("restored display command = %#v, want normal 16px text", restoredList.Commands[0])
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
