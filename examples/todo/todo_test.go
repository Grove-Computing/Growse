package todo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/yaegi"
)

func TestTodoDemoAddCompleteAndDelete(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir(".")))
	defer server.Close()

	browserState := browser.NewWithRuntimeFactory(network.NewClient(), func() runtimemodel.Runtime {
		return yaegi.New()
	})
	defer browserState.Close()
	page, err := browserState.Navigate(context.Background(), server.URL+"/index.html")
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if !page.RuntimeStarted || page.RuntimeError != "" {
		t.Fatalf("runtime state = started:%v error:%q scripts:%d script errors:%v",
			page.RuntimeStarted, page.RuntimeError, len(page.Scripts), page.ScriptErrors)
	}
	input, ok := page.Document.GetElementByID("todo-input")
	if !ok {
		t.Fatal("todo input was not found")
	}
	list, ok := page.Document.GetElementByID("todo-list")
	if !ok {
		t.Fatal("todo list was not found")
	}

	if !browserState.SubmitForm(input.ID) {
		t.Fatal("empty form submit was not handled")
	}
	if got := len(list.Children); got != 0 {
		t.Fatalf("todo count after empty submit = %d, want 0", got)
	}

	if !browserState.SetInputValue(input.ID, "Write integration test") {
		t.Fatal("SetInputValue() = false, want true")
	}
	if !browserState.SubmitForm(input.ID) {
		t.Fatal("todo form submit was not handled")
	}
	item, ok := page.Document.GetElementByID("todo-1")
	if !ok || item.TextContent() != "Write integration testCompleteDelete" {
		t.Fatalf("created todo = %#v, want item with controls", item)
	}
	if value, ok := input.Attribute("value"); !ok || value != "" {
		t.Fatalf("input value after submit = (%q, %v), want empty", value, ok)
	}

	toggle, ok := page.Document.GetElementByID("toggle-1")
	if !ok {
		t.Fatal("todo completion button was not found")
	}
	normalToggle, ok := page.ComputedStyles.For(toggle)
	if !ok || normalToggle.BackgroundColor != 0xccfbf1ff {
		t.Fatalf("normal todo button style = %#v, want default background", normalToggle)
	}
	if !browserState.UpdateHover(toggle.ID, 12, 34) {
		t.Fatal("todo completion button hover was not applied")
	}
	hoveredToggle, ok := page.ComputedStyles.For(toggle)
	if !ok || hoveredToggle.BackgroundColor != 0x99f6e4ff || hoveredToggle.Color != 0x134e4aff {
		t.Fatalf("hovered todo button style = %#v, want hover colors", hoveredToggle)
	}
	if !browserState.ClearHover() {
		t.Fatal("todo completion button hover was not cleared")
	}
	if !browserState.DispatchClick(toggle.ID, 0, 0) {
		t.Fatal("todo completion click was not handled")
	}
	if classes, ok := item.Attribute("class"); !ok || classes != "todo completed" {
		t.Fatalf("completed todo class = (%q, %v), want todo completed", classes, ok)
	}
	computed, ok := page.ComputedStyles.For(item)
	if !ok || computed.BackgroundColor != 0xd1fae5ff {
		t.Fatalf("completed todo style = %#v, want updated background", computed)
	}

	remove, ok := page.Document.GetElementByID("delete-1")
	if !ok || !browserState.DispatchClick(remove.ID, 0, 0) {
		t.Fatal("todo delete click was not handled")
	}
	if _, ok := page.Document.GetElementByID("todo-1"); ok || len(list.Children) != 0 {
		t.Fatal("deleted todo remains in the document")
	}
	if browserState.DispatchClick(toggle.ID, 0, 0) {
		t.Fatal("removed todo control still handles clicks")
	}

	for _, path := range []string{"index.html", "style.css", "_app.go"} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("demo file %s is unavailable or empty: %v", path, err)
		}
	}
}
