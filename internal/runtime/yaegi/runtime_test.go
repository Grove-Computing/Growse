package yaegi

import (
	"context"
	"strings"
	"testing"

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
