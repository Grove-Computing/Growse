package javascript

import (
	"context"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/dop251/goja"
)

func TestWindowRelationshipsPostMessageAndStructuredCloneLimits(t *testing.T) {
	document := dom.NewDocument()
	body := document.CreateElement("body", nil)
	iframe := document.CreateElement("iframe", map[string]string{"id": "child"})
	_ = document.AppendChild(document.Root, body)
	_ = document.AppendChild(body, iframe)
	self := runtimemodel.WindowReference{ID: 1, Generation: 1, Origin: "https://parent.example", URL: "https://parent.example/frame"}
	parent := runtimemodel.WindowReference{ID: 0, Generation: 1, Origin: "https://parent.example", URL: "https://parent.example/", SameOrigin: true}
	child := runtimemodel.WindowReference{ID: 2, Generation: 1, Origin: "https://child.example"}
	var messages []struct {
		target runtimemodel.WindowReference
		origin string
		data   string
	}
	var records []string
	environment := runtimemodel.Environment{
		Document: document,
		Window:   runtimemodel.WindowContext{Self: self, Parent: parent, Top: parent, Children: []runtimemodel.WindowReference{child}},
		Frames:   []runtimemodel.FrameAccess{{ID: 2, ElementID: iframe.ID, Generation: 1, Origin: child.Origin}},
		PostMessage: func(target runtimemodel.WindowReference, targetOrigin string, payload []byte) error {
			messages = append(messages, struct {
				target runtimemodel.WindowReference
				origin string
				data   string
			}{target: target, origin: targetOrigin, data: string(payload)})
			return nil
		},
		ConsoleRecord: func(_, message string) { records = append(records, message) },
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		const childElement = document.getElementById("child");
		console.log([parent === top, frames.length, frames[0] === childElement.contentWindow].join("|"));
		addEventListener("message", function(event) {
			console.log([event.origin, event.data.kind, event.data.values.join(",")].join("|"));
			event.source.postMessage({reply: true}, event.origin);
		});
		frames[0].postMessage({kind: "request", values: [1, true, null]}, "https://child.example");`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].target.ID != 2 || messages[0].origin != "https://child.example" || messages[0].data != `{"kind":"request","values":[1,true,null]}` {
		t.Fatalf("child postMessage = %#v", messages)
	}
	if err := runtime.DispatchMessage(runtimemodel.MessageEvent{
		Data: []byte(`{"kind":"response","values":[2,false]}`), Origin: "https://parent.example", Source: parent,
	}); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0] != "true|1|true" || records[1] != "https://parent.example|response|2,false" {
		t.Fatalf("message records = %v", records)
	}
	if len(messages) != 2 || messages[1].target.ID != 0 || messages[1].data != `{"reply":true}` {
		t.Fatalf("reply postMessage = %#v", messages)
	}
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		_, err := vm.RunString(`
			const cyclic = {}; cyclic.self = cyclic;
			try { postMessage(cyclic, "*"); } catch (error) { console.log("cycle:" + error.name); }
			try { postMessage("x".repeat(1048577), "*"); } catch (error) { console.log("size:" + error.name); }`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(records[2:], "|"); got != "cycle:TypeError|size:TypeError" {
		t.Fatalf("structured clone rejection = %q", got)
	}
}
