package runtime

import (
	"context"
	"testing"
)

type testRuntime struct{}

func (*testRuntime) Load(context.Context, []Script, Environment) error { return nil }
func (*testRuntime) Start(context.Context) error                       { return nil }
func (*testRuntime) Stop() error                                       { return nil }

func TestEngineNormalizationPreservesGoZeroValue(t *testing.T) {
	if got := NormalizeEngine(""); got != EngineGo {
		t.Fatalf("NormalizeEngine(\"\") = %q, want %q", got, EngineGo)
	}
	for _, engine := range []Engine{"", EngineGo, EngineJavaScript} {
		if !engine.Valid() {
			t.Fatalf("Engine %q should be valid", engine)
		}
	}
	if Engine("unknown").Valid() {
		t.Fatal("unknown Engine should be invalid")
	}
}

func TestForGoKeepsLegacyFactoryAndRejectsJavaScript(t *testing.T) {
	created := 0
	factory := ForGo(func() Runtime {
		created++
		return &testRuntime{}
	})
	if factory("") == nil || factory(EngineGo) == nil {
		t.Fatal("legacy Go factory did not create a Go Runtime")
	}
	if factory(EngineJavaScript) != nil {
		t.Fatal("legacy Go factory created a JavaScript Runtime")
	}
	if created != 2 {
		t.Fatalf("legacy factory calls = %d, want 2", created)
	}
}

func TestForGoHandlesNilFactory(t *testing.T) {
	if runtime := ForGo(nil)(EngineGo); runtime != nil {
		t.Fatalf("ForGo(nil) = %T, want nil", runtime)
	}
}
