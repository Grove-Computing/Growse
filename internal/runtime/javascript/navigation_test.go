package javascript

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/dop251/goja"
)

func TestLocationAndHistoryUseBrowserNavigationHandlers(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test:8443/app/page?old=1#start")
	currentState := `{"initial":true}`
	historyLength := 3
	var pushedState, replacedState string
	var pushedURL, replacedURL, assignedURL *url.URL
	var traversals []int
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		BaseURL: baseURL,
		Navigate: func(target *url.URL) error {
			assignedURL = target
			return nil
		},
		HistoryPush: func(state string, target *url.URL) error {
			pushedState, pushedURL, currentState = state, target, state
			historyLength++
			return nil
		},
		HistoryReplace: func(state string, target *url.URL) error {
			replacedState, replacedURL, currentState = state, target, state
			return nil
		},
		HistoryTraverse: func(delta int) error {
			traversals = append(traversals, delta)
			return nil
		},
		HistoryInfo: func() (int, string) { return historyLength, currentState },
	}
	source := `
		var initialLocation = [location.href, location.origin, location.protocol, location.host, location.hostname, location.port, location.pathname, location.search, location.hash].join("|");
		history.pushState({page: 1}, "ignored", "/next?x=1#one");
		var pushedLocation = location.href;
		var pushedPage = history.state.page;
		history.replaceState({page: 2}, "ignored", "#two");
		var replacedPage = history.state.page;
		var lengthAfterPush = history.length;
		history.back(); history.forward(); history.go(-2); history.go(0);
		location.assign("/full");
		var rejected = [];
		try { history.pushState({}, "", "https://other.test/page"); } catch (error) { rejected.push("origin"); }
		try { history.pushState({large: Array(65538).join("x")}, "", ""); } catch (error) { rejected.push("size"); }
		var cyclic = {}; cyclic.self = cyclic;
		try { history.replaceState(cyclic, "", ""); } catch (error) { rejected.push("json"); }
		try { location.assign("https://user:secret@example.test/page"); } catch (error) { rejected.push("credential"); }`
	startJavaScriptRuntime(t, runtime, source, environment)

	var initial, pushedLocation string
	var pushedPage, replacedPage, length int64
	var rejected []string
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		initial = vm.Get("initialLocation").String()
		pushedLocation = vm.Get("pushedLocation").String()
		pushedPage = vm.Get("pushedPage").ToInteger()
		replacedPage = vm.Get("replacedPage").ToInteger()
		length = vm.Get("lengthAfterPush").ToInteger()
		return vm.ExportTo(vm.Get("rejected"), &rejected)
	}); err != nil {
		t.Fatalf("read Navigation results: %v", err)
	}
	wantInitial := "https://example.test:8443/app/page?old=1#start|https://example.test:8443|https:|example.test:8443|example.test|8443|/app/page|?old=1|#start"
	if initial != wantInitial {
		t.Fatalf("initial location = %q, want %q", initial, wantInitial)
	}
	if pushedLocation != "https://example.test:8443/next?x=1#one" || pushedPage != 1 || replacedPage != 2 || length != 4 {
		t.Fatalf("History state = location:%q pushed:%d replaced:%d length:%d", pushedLocation, pushedPage, replacedPage, length)
	}
	if pushedState != `{"page":1}` || pushedURL.String() != "https://example.test:8443/next?x=1#one" ||
		replacedState != `{"page":2}` || replacedURL.String() != "https://example.test:8443/next?x=1#two" {
		t.Fatalf("Browser History handlers = push(%q,%v) replace(%q,%v)", pushedState, pushedURL, replacedState, replacedURL)
	}
	if assignedURL == nil || assignedURL.String() != "https://example.test:8443/full" {
		t.Fatalf("location.assign target = %v", assignedURL)
	}
	if got, want := traversals, []int{-1, 1, -2}; len(got) != len(want) || got[0] != -1 || got[1] != 1 || got[2] != -2 {
		t.Fatalf("history traversals = %v, want %v", got, want)
	}
	if strings.Join(rejected, ",") != "origin,size,json,credential" {
		t.Fatalf("rejected Navigation cases = %v", rejected)
	}
}

func TestNavigationEventsRunOnPageQueueAndIsolateExceptions(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page#old")
	messages := make(chan string, 4)
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		BaseURL: baseURL,
		ConsoleRecord: func(_, message string) {
			messages <- message
		},
	}
	source := `
		var popCalls = 0;
		function popHandler(event) { popCalls += 1; console.log("pop:" + event.state.page); }
		addEventListener("popstate", popHandler);
		addEventListener("popstate", popHandler);
		addEventListener("hashchange", function (event) { console.log("hash:" + event.oldURL + "->" + event.newURL); throw new Error("hash listener failure"); });`
	startJavaScriptRuntime(t, runtime, source, environment)
	runtime.DispatchPopState(`{"page":2}`)
	runtime.DispatchHashChange("https://example.test/page#old", "https://example.test/page#new")
	if err := runtime.runSync(context.Background(), func(*goja.Runtime) error { return nil }); err != nil {
		t.Fatalf("Page queue barrier: %v", err)
	}

	got := []string{receiveMessage(t, messages), receiveMessage(t, messages), receiveMessage(t, messages)}
	joined := strings.Join(got, "\n")
	for _, expected := range []string{"pop:2", "hash:https://example.test/page#old->https://example.test/page#new", "hash listener failure"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("Navigation Event records = %v, missing %q", got, expected)
		}
	}
	var popCalls int64
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		popCalls = vm.Get("popCalls").ToInteger()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if popCalls != 1 {
		t.Fatalf("duplicate popstate calls = %d, want 1", popCalls)
	}
}

func TestNavigationHandlerFailureDoesNotCommitJavaScriptLocation(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page")
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		BaseURL: baseURL,
		HistoryPush: func(string, *url.URL) error {
			return errors.New("history capacity exceeded")
		},
	}
	startJavaScriptRuntime(t, runtime, `
		var failed = false;
		try { history.pushState({page: 2}, "", "/next"); } catch (error) { failed = error.message.indexOf("capacity") >= 0; }
		var hrefAfterFailure = location.href;`, environment)
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		if !vm.Get("failed").ToBoolean() || vm.Get("hrefAfterFailure").String() != baseURL.String() {
			t.Fatal("failed History mutation changed JavaScript location")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
