package counter

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

func TestCounterDemoFiles(t *testing.T) {
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
	button, ok := page.Document.GetElementByID("increment")
	if !ok {
		t.Fatal("increment button was not found")
	}
	output, ok := page.Document.GetElementByID("count")
	if !ok {
		t.Fatal("count output was not found")
	}
	normalButton, ok := page.ComputedStyles.For(button)
	if !ok || normalButton.BackgroundColor != 0xdbeafeff {
		t.Fatalf("normal button style = %#v, want default background", normalButton)
	}
	if !browserState.UpdateHover(button.ID, 12, 34) {
		t.Fatal("button hover was not applied")
	}
	hoveredButton, ok := page.ComputedStyles.For(button)
	if !ok || hoveredButton.BackgroundColor != 0xbae6fdff || hoveredButton.Color != 0x0c4a6eff {
		t.Fatalf("hovered button style = %#v, want hover colors", hoveredButton)
	}
	if !browserState.ClearHover() {
		t.Fatal("button hover was not cleared")
	}
	restoredButton, _ := page.ComputedStyles.For(button)
	if restoredButton.BackgroundColor != normalButton.BackgroundColor {
		t.Fatalf("restored button background = %#x, want %#x", restoredButton.BackgroundColor, normalButton.BackgroundColor)
	}
	for click := 1; click <= 2; click++ {
		if !browserState.DispatchClick(button.ID, 0, 0) {
			t.Fatalf("click %d was not handled", click)
		}
	}
	if got, want := output.TextContent(), "2"; got != want {
		t.Fatalf("counter text = %q, want %q", got, want)
	}

	for _, path := range []string{"index.html", "style.css", "_app.go"} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("demo file %s is unavailable or empty: %v", path, err)
		}
	}
}
