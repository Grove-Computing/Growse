package javascript

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestInitialScriptLimitFailsOnlyTheTargetRuntime(t *testing.T) {
	scripts := make([]runtimemodel.Script, maxPageScripts+1)
	for index := range scripts {
		scripts[index] = javaScript(fmt.Sprintf("globalThis.value%d = true", index))
	}
	overflow := New()
	if err := overflow.Load(context.Background(), scripts, runtimemodel.Environment{}); err == nil || !strings.Contains(err.Error(), "scripts") {
		t.Fatalf("overflow Load() error = %v", err)
	}

	var record string
	other := New()
	t.Cleanup(func() { _ = other.Stop() })
	if err := other.Load(context.Background(), []runtimemodel.Script{javaScript(`console.log("other-page-ran")`)}, runtimemodel.Environment{
		ConsoleRecord: func(_, message string) { record = message },
	}); err != nil {
		t.Fatal(err)
	}
	if err := other.Start(context.Background()); err != nil || record != "other-page-ran" {
		t.Fatalf("independent Runtime = error:%v record:%q", err, record)
	}
}

func TestDynamicScriptCountAndInsertionDepthAreFinite(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		document, _ := htmlparser.Parse(strings.NewReader(`<main id="host"></main><output id="result">idle</output>`))
		var errors int
		runtime := New()
		t.Cleanup(func() { _ = runtime.Stop() })
		source := fmt.Sprintf(`
			var host = document.getElementById("host");
			for (var index = 0; index < %d; index++) { host.appendChild(document.createElement("script")); }
			document.getElementById("result").textContent = "continued";`, maxPageScripts+20)
		environment := runtimemodel.Environment{
			Document: document, Events: events.NewDispatcher(),
			ConsoleRecord: func(level, _ string) {
				if level == "error" {
					errors++
				}
			},
		}
		if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
			t.Fatal(err)
		}
		if err := runtime.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		result, _ := document.GetElementByID("result")
		if result.TextContent() != "continued" || runtime.scriptCount != maxPageScripts || len(runtime.dynamicScripts) != maxPageScripts-1 || errors == 0 {
			t.Fatalf("count limit = text:%q count:%d prepared:%d errors:%d", result.TextContent(), runtime.scriptCount, len(runtime.dynamicScripts), errors)
		}
	})

	t.Run("insertion-depth", func(t *testing.T) {
		document, _ := htmlparser.Parse(strings.NewReader(`<main id="host"></main><output id="result">idle</output>`))
		var records [][2]string
		runtime := New()
		t.Cleanup(func() { _ = runtime.Stop() })
		source := `
			var host = document.getElementById("host");
			function insert(depth) {
				var child = document.createElement("script");
				child.textContent = "insert(" + String(depth + 1) + ")";
				host.appendChild(child);
			}
			insert(0);
			document.getElementById("result").textContent = "continued";`
		if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, runtimemodel.Environment{
			Document: document, Events: events.NewDispatcher(),
			ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) },
		}); err != nil {
			t.Fatal(err)
		}
		if err := runtime.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		result, _ := document.GetElementByID("result")
		if result.TextContent() != "continued" || len(records) != 1 || !strings.Contains(records[0][1], "depth") || runtime.dynamicInsertDepth != 0 {
			t.Fatalf("insertion limit = text:%q records:%v depth:%d", result.TextContent(), records, runtime.dynamicInsertDepth)
		}
	})
}

func TestResourceReprepareCountAndFailureRetryLimits(t *testing.T) {
	runtime := &Runtime{
		resourcePrepareCounts: make(map[uint64]int), resourceFailures: make(map[string]int),
		stylesheetStates: make(map[uint64]string),
	}
	for attempt := range maxResourceReprepares {
		changed, err := runtime.updateResourceSignature(runtime.stylesheetStates, 1, fmt.Sprint(attempt), maxDynamicStylesheets)
		if !changed || err != nil {
			t.Fatalf("prepare %d = changed:%t error:%v", attempt, changed, err)
		}
	}
	if changed, err := runtime.updateResourceSignature(runtime.stylesheetStates, 1, "overflow", maxDynamicStylesheets); changed || err == nil {
		t.Fatalf("overflow prepare = changed:%t error:%v", changed, err)
	}
	for range maxResourceFailureRetries {
		if !runtime.allowResourceAttempt("https://example.test/fail") {
			t.Fatal("resource retry stopped too early")
		}
		runtime.recordResourceFailure("https://example.test/fail", fmt.Errorf("failed"))
	}
	if runtime.allowResourceAttempt("https://example.test/fail") {
		t.Fatal("resource retry remained unbounded")
	}

	runtime.scriptBytes = maxPageScriptBytes - 1
	if !runtime.reserveScriptBytes(1) || runtime.reserveScriptBytes(1) {
		t.Fatal("Page script byte budget was not enforced exactly")
	}
}
