package javascript

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestHydrationTaskLimitsBecomeCatchablePageErrors(t *testing.T) {
	defaults := New()
	if defaults.maxListeners != 20_000 || defaults.maxObservers != 2_048 || defaults.maxObserverRecords != 16_384 || defaults.maxObserverCallbacks != 1_024 || defaults.maxMutationsPerTask != 100_000 || defaults.maxForcedReadsPerTask != 1_024 {
		t.Fatalf("hydration safety defaults = listeners:%d observers:%d records:%d callbacks:%d mutations:%d reads:%d",
			defaults.maxListeners, defaults.maxObservers, defaults.maxObserverRecords, defaults.maxObserverCallbacks, defaults.maxMutationsPerTask, defaults.maxForcedReadsPerTask)
	}
	document, err := htmlparser.Parse(strings.NewReader(`<main id="target">ready</main>`))
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	runtime := New()
	runtime.maxMutationsPerTask = 2
	runtime.maxForcedReadsPerTask = 2
	runtime.maxListeners = 2
	runtime.maxObservers = 2
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(),
		ReadRender: func(context.Context, dom.NodeID) (runtimemodel.RenderSnapshot, error) {
			return runtimemodel.RenderSnapshot{Revision: 1, Rect: runtimemodel.DOMRect{Width: 10, Height: 10}, ClientWidth: 10}, nil
		},
		ConsoleRecord: func(_, value string) { messages = append(messages, value) },
	}
	source := `
		const target = document.getElementById("target");
		const errors = [];
		try { target.textContent = "x".repeat(1024 * 1024 + 1); } catch (error) { errors.push("string"); }
		target.setAttribute("data-a", "1"); target.setAttribute("data-b", "2");
		try { target.setAttribute("data-c", "3"); } catch (error) { errors.push("mutation"); }
		void target.clientWidth; void target.clientWidth;
		try { void target.clientWidth; } catch (error) { errors.push("layout"); }
		target.addEventListener("one", function() {}); target.addEventListener("two", function() {});
		try { target.addEventListener("three", function() {}); } catch (error) { errors.push("listener"); }
		new MutationObserver(function() {}); new ResizeObserver(function() {});
		try { new IntersectionObserver(function() {}); } catch (error) { errors.push("observer"); }
		console.log(errors.join(","));`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(messages, []string{"string,mutation,layout,listener,observer"}) {
		t.Fatalf("hydration limit errors = %#v", messages)
	}

	var secondMessage string
	second := New()
	t.Cleanup(func() { _ = second.Stop() })
	if err := second.Load(context.Background(), []runtimemodel.Script{javaScript(`console.log("second-page-running")`)}, runtimemodel.Environment{ConsoleRecord: func(_, value string) { secondMessage = value }}); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if secondMessage != "second-page-running" {
		t.Fatalf("second Page result = %q", secondMessage)
	}
}
