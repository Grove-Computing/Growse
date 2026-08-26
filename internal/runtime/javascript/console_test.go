package javascript

import (
	"context"
	"testing"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestConsoleRecordsLevelsAndSafeValues(t *testing.T) {
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	var records [][2]string
	environment := runtimemodel.Environment{ConsoleRecord: func(level, message string) {
		records = append(records, [2]string{level, message})
	}}
	source := `
		var getterCalled = false;
		var object = {};
		Object.defineProperty(object, "secret", { get: function () {
			getterCalled = true;
			throw new Error("getter must not run");
		}});
		console.log("text", 42, true, null, undefined, object, [1], function () {});
		console.info("info");
		console.warn("warn");
		console.error("error");
		if (getterCalled) { throw new Error("console evaluated object getter"); }`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	want := [][2]string{
		{"log", "text 42 true null undefined [object Object] [object Array] [object Function]"},
		{"info", "info"},
		{"warn", "warn"},
		{"error", "error"},
	}
	if !equalRecords(records, want) {
		t.Fatalf("console records = %v, want %v", records, want)
	}
}

func TestConsoleUsesLegacyLogCallbackWhenStructuredRecorderIsAbsent(t *testing.T) {
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	var message string
	environment := runtimemodel.Environment{ConsoleLog: func(value string) { message = value }}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(`console.warn("legacy", 7)`)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := message, "legacy 7"; got != want {
		t.Fatalf("ConsoleLog message = %q, want %q", got, want)
	}
}
