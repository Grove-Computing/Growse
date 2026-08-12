package counter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/saku0512/growse/internal/browser"
	"github.com/saku0512/growse/internal/network"
	runtimemodel "github.com/saku0512/growse/internal/runtime"
	"github.com/saku0512/growse/internal/runtime/yaegi"
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
