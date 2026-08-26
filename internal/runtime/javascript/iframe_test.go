package javascript

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/dop251/goja"
)

func TestIframeSameOriginAccessCrossOriginProxyAndStaleGeneration(t *testing.T) {
	parent := dom.NewDocument()
	body := parent.CreateElement("body", nil)
	sameElement := parent.CreateElement("iframe", map[string]string{"id": "same"})
	crossElement := parent.CreateElement("iframe", map[string]string{"id": "cross"})
	_ = parent.AppendChild(parent.Root, body)
	_ = parent.AppendChild(body, sameElement)
	_ = parent.AppendChild(body, crossElement)
	child := dom.NewDocument()
	childBody := child.CreateElement("body", nil)
	result := child.CreateElement("p", map[string]string{"id": "result"})
	_ = child.AppendChild(child.Root, childBody)
	_ = child.AppendChild(childBody, result)
	_ = child.SetTextContent(result.ID, "before")
	pageURL, _ := url.Parse("https://page.example/index.html")
	var mutations int
	var records []string
	environment := runtimemodel.Environment{
		Document: parent, BaseURL: pageURL,
		Frames: []runtimemodel.FrameAccess{
			{ID: 1, ElementID: sameElement.ID, Generation: 1, Origin: "https://page.example", URL: "https://page.example/frame.html", SameOrigin: true, Document: child},
			{ID: 2, ElementID: crossElement.ID, Generation: 1, Origin: "https://other.example", SameOrigin: false},
		},
		FrameMutation: func(frameID, generation uint64, snapshot dom.DocumentSnapshot) error {
			if frameID != 1 || generation != 1 {
				return fmt.Errorf("unexpected Frame mutation %d/%d", frameID, generation)
			}
			mutations++
			return child.ApplySnapshot(snapshot)
		},
		ConsoleRecord: func(_, message string) { records = append(records, message) },
	}
	source := `
		var sameFrame = document.getElementById("same");
		var oldWindow = sameFrame.contentWindow;
		var oldDocument = sameFrame.contentDocument;
		oldDocument.getElementById("result").textContent = "after";
		var crossFrame = document.getElementById("cross");
		console.log([
			oldWindow.document.getElementById("result").textContent,
			oldWindow.location.origin,
			crossFrame.contentDocument === null,
			typeof crossFrame.contentWindow.document,
			typeof crossFrame.contentWindow.location
		].join("|"));`
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if mutations != 1 || result.TextContent() != "after" || len(records) != 1 || records[0] != "after|https://page.example|true|undefined|undefined" {
		t.Fatalf("Frame access = mutations:%d text:%q records:%v", mutations, result.TextContent(), records)
	}

	runtime.UpdateFrames([]runtimemodel.FrameAccess{{ID: 1, ElementID: sameElement.ID, Generation: 2, SameOrigin: false}})
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		_, err := vm.RunString(`
			for (const stale of [function () { return oldWindow.document; }, function () { return oldDocument.getElementById("result"); }]) {
				try { stale(); console.log("stale:allowed"); } catch (error) { console.log("stale:" + error.name); }
			}`)
		return err
	}); err != nil {
		t.Fatalf("stale access script error = %v", err)
	}
	if len(records) != 3 || records[1] != "stale:SecurityError" || records[2] != "stale:SecurityError" {
		t.Fatalf("stale Frame records = %v", records)
	}
}
