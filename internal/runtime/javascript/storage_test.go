package javascript

import (
	"context"
	"errors"
	"strings"
	"testing"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
	"github.com/dop251/goja"
)

func TestStorageSynchronousOperationsAndQuota(t *testing.T) {
	local := storagecore.NewArea()
	session := storagecore.NewArea()
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		localStorage.setItem("first", "one");
		localStorage.setItem("second", 2);
		var firstKey = localStorage.key(0);
		var secondValue = localStorage.getItem("second");
		var missing = localStorage.getItem("missing");
		localStorage.removeItem("first");
		sessionStorage.setItem("tab", "isolated");
		var quotaRejected = false;
		try { localStorage.setItem("large", Array(1024 * 1024 + 2).join("x")); }
		catch (error) { quotaRejected = error.message.indexOf("quota") >= 0; }`
	environment := runtimemodel.Environment{
		LocalStorage: local, SessionStorage: session,
		StorageSource: storagecore.MutationSource{ID: 1, URL: "https://example.test/page"},
	}
	startJavaScriptRuntime(t, runtime, source, environment)
	var firstKey, secondValue string
	var missingNull, quotaRejected bool
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		firstKey = vm.Get("firstKey").String()
		secondValue = vm.Get("secondValue").String()
		missingNull = goja.IsNull(vm.Get("missing"))
		quotaRejected = vm.Get("quotaRejected").ToBoolean()
		return nil
	}); err != nil {
		t.Fatalf("read Storage results: %v", err)
	}
	if firstKey != "first" || secondValue != "2" || !missingNull || !quotaRejected {
		t.Fatalf("Storage results = key:%q value:%q missingNull:%v quota:%v", firstKey, secondValue, missingNull, quotaRejected)
	}
	if local.Len() != 1 || session.Len() != 1 {
		t.Fatalf("Storage lengths = local:%d session:%d, want 1/1", local.Len(), session.Len())
	}
	if value, ok := session.Get("tab"); !ok || value != "isolated" {
		t.Fatalf("Session Storage tab = (%q, %v)", value, ok)
	}
}

func TestStorageFailureRollsBackAndReachesJavaScript(t *testing.T) {
	local := storagecore.NewFailedArea(errors.New("profile commit failed"))
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{LocalStorage: local, SessionStorage: storagecore.NewArea()}
	startJavaScriptRuntime(t, runtime, `
		var failure = "";
		try { localStorage.setItem("key", "value"); } catch (error) { failure = error.message; }`, environment)
	var failure string
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		failure = vm.Get("failure").String()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(failure, "profile commit failed") || local.Len() != 0 {
		t.Fatalf("failure = %q, local length = %d", failure, local.Len())
	}
}

func TestLocalStorageEventIsTabScopedAndCloseUnsubscribes(t *testing.T) {
	local := storagecore.NewArea()
	firstSession := storagecore.NewArea()
	secondSession := storagecore.NewArea()
	firstMessages := make(chan string, 1)
	secondMessages := make(chan string, 2)

	second := New()
	startJavaScriptRuntime(t, second, `
		addEventListener("storage", function (event) {
			console.log([event.key, event.oldValue, event.newValue, event.url, event.storageArea === localStorage].join("|"));
		});`, runtimemodel.Environment{
		LocalStorage: local, SessionStorage: secondSession,
		StorageSource: storagecore.MutationSource{ID: 2, URL: "https://example.test/second"},
		ConsoleRecord: func(_, message string) { secondMessages <- message },
	})

	first := New()
	startJavaScriptRuntime(t, first, `
		addEventListener("storage", function () { console.log("self event"); });
		sessionStorage.setItem("tab", "first only");
		localStorage.setItem("mode", "dark");`, runtimemodel.Environment{
		LocalStorage: local, SessionStorage: firstSession,
		StorageSource: storagecore.MutationSource{ID: 1, URL: "https://example.test/first"},
		ConsoleRecord: func(_, message string) { firstMessages <- message },
	})
	t.Cleanup(func() { _ = first.Stop() })

	if got, want := receiveMessage(t, secondMessages), "mode||dark|https://example.test/first|true"; got != want {
		t.Fatalf("storage event = %q, want %q", got, want)
	}
	select {
	case message := <-firstMessages:
		t.Fatalf("updating tab received its own Storage event: %q", message)
	default:
	}
	if _, exists := secondSession.Get("tab"); exists {
		t.Fatal("Session Storage leaked to another Tab")
	}
	if err := second.Stop(); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
	if err := local.SetFrom(storagecore.MutationSource{ID: 1, URL: "https://example.test/first"}, "mode", "light"); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-secondMessages:
		t.Fatalf("closed Page received Storage event: %q", message)
	default:
	}

	restored := New()
	t.Cleanup(func() { _ = restored.Stop() })
	startJavaScriptRuntime(t, restored, `var restoredMode = localStorage.getItem("mode");`, runtimemodel.Environment{
		LocalStorage: local, SessionStorage: storagecore.NewArea(),
		StorageSource: storagecore.MutationSource{ID: 3, URL: "https://example.test/restored"},
	})
	var mode string
	if err := restored.runSync(context.Background(), func(vm *goja.Runtime) error {
		mode = vm.Get("restoredMode").String()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if mode != "light" {
		t.Fatalf("restored localStorage mode = %q, want light", mode)
	}
}
