package isolated

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/workerproto"
)

func TestMinimalWorkerEnvironmentRejectsUnexpectedNames(t *testing.T) {
	valid := []string{workerEnvironmentKey + "=1", "GOMAXPROCS=1"}
	if runtime.GOOS == "windows" {
		valid = append(valid, "SystemRoot=C:\\Windows")
	}
	if !hasMinimalWorkerEnvironment(valid) {
		t.Fatalf("worker environment was rejected: %v", valid)
	}
	if hasMinimalWorkerEnvironment(append(valid, "HOME=/private")) {
		t.Fatal("unexpected worker environment was accepted")
	}
}

func TestSandboxStatusValidationFailsClosed(t *testing.T) {
	valid := sandboxStatusResponse{
		SandboxStatus: runtimemodel.SandboxStatus{
			Platform: runtime.GOOS, Ready: true, ProcessBoundary: true, BrokeredHostIO: true,
			MinimalEnvironment: true, ParentLifecycle: true, MemoryLimitBytes: maxWorkerHeapBytes,
		},
		ProfileVersion: sandboxProfileVersion, ProtocolVersion: workerproto.Version, WorkerPID: 202,
	}
	if err := validateSandboxStatus(valid, 101); err != nil {
		t.Fatalf("valid sandbox status = %v", err)
	}
	cases := map[string]func(*sandboxStatusResponse){
		"profile":      func(status *sandboxStatusResponse) { status.ProfileVersion++ },
		"protocol":     func(status *sandboxStatusResponse) { status.ProtocolVersion++ },
		"platform":     func(status *sandboxStatusResponse) { status.Platform = "unknown" },
		"same process": func(status *sandboxStatusResponse) { status.WorkerPID = 101 },
		"process":      func(status *sandboxStatusResponse) { status.ProcessBoundary = false },
		"broker":       func(status *sandboxStatusResponse) { status.BrokeredHostIO = false },
		"environment":  func(status *sandboxStatusResponse) { status.MinimalEnvironment = false },
		"lifecycle":    func(status *sandboxStatusResponse) { status.ParentLifecycle = false },
		"memory":       func(status *sandboxStatusResponse) { status.MemoryLimitBytes = 0 },
		"ready":        func(status *sandboxStatusResponse) { status.Ready = false },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			status := valid
			mutate(&status)
			if err := validateSandboxStatus(status, 101); err == nil {
				t.Fatal("invalid sandbox status was accepted")
			}
		})
	}
}

func TestRuntimeReportsVerifiedPlatformSandbox(t *testing.T) {
	document, _ := htmlparser.Parse(strings.NewReader(`<p>safe</p>`))
	pageRuntime := New(runtimemodel.EngineJavaScript)
	t.Cleanup(func() { _ = pageRuntime.Stop() })
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(), BaseURL: mustURL(t, "https://example.test/"),
	}
	script := runtimemodel.Script{Engine: runtimemodel.EngineJavaScript, SourceURL: environment.BaseURL, Source: `void 0;`}
	if err := pageRuntime.Load(context.Background(), []runtimemodel.Script{script}, environment); err != nil {
		t.Fatal(err)
	}
	status := pageRuntime.SandboxStatus()
	if !status.Ready || status.Platform != runtime.GOOS || !status.ProcessBoundary || !status.BrokeredHostIO ||
		!status.MinimalEnvironment || !status.ParentLifecycle || status.MemoryLimitBytes != maxWorkerHeapBytes {
		t.Fatalf("sandbox status = %#v", status)
	}
	if len(status.Constraints) == 0 || os.Getpid() <= 0 {
		t.Fatalf("sandbox constraints = %v", status.Constraints)
	}
}
