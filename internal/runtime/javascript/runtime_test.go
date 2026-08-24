package javascript

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestRuntimeEvaluatesScriptsInDocumentOrder(t *testing.T) {
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	var records [][2]string
	environment := runtimemodel.Environment{ConsoleRecord: func(level, message string) {
		records = append(records, [2]string{level, message})
	}}
	scripts := []runtimemodel.Script{
		javaScript(`var order = ["first"];`),
		javaScript(`order.push("second"); console.log(order.join(","));`),
	}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := records, [][2]string{{"log", "first,second"}}; !equalRecords(got, want) {
		t.Fatalf("console records = %v, want %v", got, want)
	}
}

func TestRuntimeContainsSyntaxAndRuntimeErrors(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "syntax", source: `function (`},
		{name: "runtime", source: `throw new Error("page failure")`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := New()
			t.Cleanup(func() { _ = runtime.Stop() })
			if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(test.source)}, runtimemodel.Environment{}); err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if err := runtime.Start(context.Background()); err == nil {
				t.Fatal("Start() error = nil, want page-scoped JavaScript error")
			}
		})
	}

	good := New()
	t.Cleanup(func() { _ = good.Stop() })
	if err := good.Load(context.Background(), []runtimemodel.Script{javaScript(`console.log("still alive")`)}, runtimemodel.Environment{}); err != nil {
		t.Fatalf("independent Load() error = %v", err)
	}
	if err := good.Start(context.Background()); err != nil {
		t.Fatalf("independent Start() error = %v", err)
	}
}

func TestRuntimeInterruptsRunningScriptWhenPageIsCanceled(t *testing.T) {
	pageContext, cancelPage := context.WithCancel(context.Background())
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	if err := runtime.Load(pageContext, []runtimemodel.Script{javaScript(`for (;;) {}`)}, runtimemodel.Environment{}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	finished := make(chan error, 1)
	go func() { finished <- runtime.Start(context.Background()) }()
	time.Sleep(10 * time.Millisecond)
	cancelPage()

	select {
	case err := <-finished:
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("Start() error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start() did not stop after Page cancellation")
	}
}

func TestRuntimeRejectsWrongEngineAndUseAfterStop(t *testing.T) {
	runtime := New()
	if err := runtime.Load(context.Background(), []runtimemodel.Script{{Engine: runtimemodel.EngineGo, Source: "package main"}}, runtimemodel.Environment{}); err == nil {
		t.Fatal("Load() error = nil, want engine mismatch")
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(`true`)}, runtimemodel.Environment{}); !errors.Is(err, errRuntimeStopped) {
		t.Fatalf("Load() error = %v, want errRuntimeStopped", err)
	}
}

func javaScript(source string) runtimemodel.Script {
	return runtimemodel.Script{Engine: runtimemodel.EngineJavaScript, Source: source, Inline: true}
}

func equalRecords(left, right [][2]string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
