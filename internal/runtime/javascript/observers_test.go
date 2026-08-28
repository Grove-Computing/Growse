package javascript

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/dop251/goja"
)

func TestMutationObserverDeliversBoundedRecordsAtCheckpoint(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main id="host"><p id="target" class="old">old</p></main>`))
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		const target = document.getElementById("target");
		const observer = new MutationObserver(function(records) {
			console.log(records.map(function(record) {
				return record.type + ":" + (record.attributeName || "") + ":" + (record.oldValue || "") + ":" + record.addedNodes.length;
			}).join(","));
		});
		observer.observe(target, { attributes: true, attributeOldValue: true, characterData: true, characterDataOldValue: true, childList: true, subtree: true });
		target.setAttribute("class", "new");
		target.firstChild.textContent = "new";
		target.appendChild(document.createElement("span"));`
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher(), ConsoleRecord: func(_, value string) { messages = append(messages, value) }}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"attributes:class:old:0,characterData::old:0,childList:::1"}
	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("MutationObserver records = %#v, want %#v", messages, want)
	}
}

func TestResizeAndIntersectionObserversRunAfterFrame(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main id="target">content</main>`))
	if err != nil {
		t.Fatal(err)
	}
	target, _ := document.GetElementByID("target")
	var messages []string
	frames := 0
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(),
		Media:        runtimemodel.MediaEnvironment{ViewportWidth: 100, ViewportHeight: 100, ColorScheme: "light", Hover: true, Pointer: "fine"},
		RequestFrame: func() { frames++ }, ConsoleRecord: func(_, value string) { messages = append(messages, value) },
		ReadRender: func(_ context.Context, nodeID dom.NodeID) (runtimemodel.RenderSnapshot, error) {
			x, width := float32(90), float32(20)
			if value, _ := target.Attribute("data-state"); value == "changed" {
				x, width = 200, 40
			}
			return runtimemodel.RenderSnapshot{Revision: 3, Rect: runtimemodel.DOMRect{X: x, Y: 10, Width: width, Height: 20}, ClientWidth: width}, nil
		},
	}
	source := `
		const target = document.getElementById("target");
		new ResizeObserver(function(entries) { console.log("resize:" + entries[0].contentRect.width); }).observe(target);
		new IntersectionObserver(function(entries) { console.log("intersection:" + entries[0].isIntersecting + ":" + entries[0].intersectionRatio); }, { threshold: [0, 0.5] }).observe(target);`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if frames != 2 || !runtime.HasAnimationFrameCallbacks() || !runtime.RunAnimationFrame(time.Unix(1, 0)) {
		t.Fatalf("initial observer frame = requests:%d pending:%t", frames, runtime.HasAnimationFrameCallbacks())
	}
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		_, err := vm.RunString(`target.setAttribute("data-state", "changed")`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !runtime.RunAnimationFrame(time.Unix(2, 0)) {
		t.Fatal("changed observer frame was not delivered")
	}
	want := []string{"resize:20", "intersection:true:0.5", "resize:40", "intersection:false:0"}
	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("Frame observer records = %#v, want %#v", messages, want)
	}
}

func TestObserverMutationLoopBecomesFiniteRuntimeError(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main id="target"></main>`))
	if err != nil {
		t.Fatal(err)
	}
	var messages [][2]string
	runtime := New()
	runtime.maxObserverCallbacks = 3
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		const target = document.getElementById("target");
		let count = 0;
		new MutationObserver(function() { count++; target.setAttribute("data-loop", String(count)); }).observe(target, { attributes: true });
		target.setAttribute("data-loop", "start");`
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher(), ConsoleRecord: func(level, value string) { messages = append(messages, [2]string{level, value}) }}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0][0] != "error" || !strings.Contains(messages[0][1], "checkpoint callback limit") {
		t.Fatalf("observer loop result = %#v", messages)
	}
	if runtime.pendingMutationRecords != 0 {
		t.Fatalf("pending mutation records = %d", runtime.pendingMutationRecords)
	}
}

func TestObserverRecordLimitBecomesFiniteRuntimeError(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main id="target"></main>`))
	if err != nil {
		t.Fatal(err)
	}
	var messages [][2]string
	runtime := New()
	runtime.maxObserverRecords = 2
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		const target = document.getElementById("target");
		new MutationObserver(function() {}).observe(target, { attributes: true });
		target.setAttribute("data-a", "1");
		target.setAttribute("data-b", "2");
		target.setAttribute("data-c", "3");`
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher(), ConsoleRecord: func(level, value string) { messages = append(messages, [2]string{level, value}) }}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0][0] != "error" || !strings.Contains(messages[0][1], "record limit") {
		t.Fatalf("observer record limit result = %#v", messages)
	}
}
