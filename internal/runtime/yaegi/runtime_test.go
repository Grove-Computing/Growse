package yaegi

import (
	"context"
	"strings"
	"testing"

	dommodel "github.com/saku0512/growse/internal/dom"
	runtimemodel "github.com/saku0512/growse/internal/runtime"
)

func TestRuntimeStartsMainAfterLoadingMultipleScripts(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{
		{Source: "package main\nvar Started bool\nfunc markStarted() { Started = true }"},
		{Source: "package main\nfunc main() { markStarted() }"},
	}

	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	packageSymbols, ok := runtime.interpreter.Symbols("page")["page"]
	if !ok {
		t.Fatal("page package symbols were not exported")
	}
	value, ok := packageSymbols["Started"]
	if !ok {
		t.Fatal("Started symbol was not exported")
	}
	if !value.Bool() {
		t.Fatal("main() was not invoked")
	}
}

func TestRuntimeExposesGrowseConsole(t *testing.T) {
	runtime := New()
	var messages []string
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/console"
func main() { console.Log("Hello from Go", 42) }`}}
	environment := runtimemodel.Environment{ConsoleLog: func(message string) {
		messages = append(messages, message)
	}}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := len(messages), 1; got != want {
		t.Fatalf("console message count = %d, want %d (%v)", got, want, messages)
	}
	if got, want := messages[0], "[WebGo] Hello from Go42"; got != want {
		t.Fatalf("console message = %q, want %q", got, want)
	}
}

func TestRuntimeExposesGrowseDOM(t *testing.T) {
	document := dommodel.NewDocument()
	message := document.CreateElement("p", map[string]string{"id": "message"})
	if err := document.AppendChild(document.Root, message); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(message, document.CreateText("before")); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	element := dom.GetElementByID("message")
	if element != nil && element.Text() == "before" {
		element.SetText("after")
	}
}`}}
	environment := runtimemodel.Environment{
		Document: document,
		OnMutation: func() {
			mutations++
		},
	}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := message.TextContent(), "after"; got != want {
		t.Fatalf("TextContent() = %q, want %q", got, want)
	}
	if mutations != 1 {
		t.Fatalf("mutation count = %d, want 1", mutations)
	}
}

func TestRuntimeLoadRequiresMainPackage(t *testing.T) {
	runtime := New()
	err := runtime.Load(context.Background(), []runtimemodel.Script{{
		Source: "package other\nfunc main() {}",
	}}, runtimemodel.Environment{})
	if err == nil || !strings.Contains(err.Error(), "want package main") {
		t.Fatalf("Load() error = %v, want package validation error", err)
	}
}

func TestRuntimeReportsMissingMain(t *testing.T) {
	runtime := New()
	err := runtime.Load(context.Background(), []runtimemodel.Script{{
		Source: "package main\nfunc helper() {}",
	}}, runtimemodel.Environment{})
	if err == nil || !strings.Contains(err.Error(), "exactly 1") {
		t.Fatalf("Load() error = %v, want missing main error", err)
	}
}

func TestRuntimeStopIsIdempotent(t *testing.T) {
	runtime := New()
	if err := runtime.Stop(); err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}
