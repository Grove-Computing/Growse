package browser

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	runtimejavascript "github.com/Grove-Computing/Growse/internal/runtime/javascript"
	runtimeyaegi "github.com/Grove-Computing/Growse/internal/runtime/yaegi"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

type runtimeStub struct {
	loadCalls        atomic.Int32
	startCalls       atomic.Int32
	stopCalls        atomic.Int32
	loadErr          error
	startErr         error
	scripts          []runtimemodel.Script
	environment      runtimemodel.Environment
	mutateOnStart    bool
	navigateOnStart  string
	popStates        []string
	hashChanges      [][2]string
	navigationEvents []string
}

func TestJavaScriptConsoleRecordRetainsSelectedEngine(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/javascript-console.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<script>console.warn("from js")</script>`),
	}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime {
		if engine == runtimemodel.EngineJavaScript {
			return runtimejavascript.New()
		}
		return runtimeyaegi.New()
	})
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatalf("SetEngine() error = %v", err)
	}
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	records := page.DevTools.Console()
	if len(records) != 1 || records[0].Engine != "javascript" || records[0].Level != devtools.ConsoleWarn ||
		records[0].Source != "console" || records[0].Message != "from js" {
		t.Fatalf("Console() = %#v, want JavaScript warn record", records)
	}
}

func TestAnimationFrameMutationUsesSharedFrameTimestamp(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/frame.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<style>
#box { opacity: 0; transition: opacity 1s linear; }
#box.active { opacity: 1; }
</style>
<div id="box"></div>
<script type="text/go">package main
import (
	"growse/dom"
	"growse/scheduler"
)
func main() {
	_, _ = scheduler.RequestAnimationFrame(func(timestamp scheduler.Timestamp) {
		_ = timestamp
		dom.GetElementByID("box").AddClass("active")
	})
}</script>`),
	}}
	start := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	clock := &browserFakeClock{current: start}
	browserState := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return runtimeyaegi.New() })
	browserState.SetAnimationClock(clock)

	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	frameTime := start.Add(100 * time.Millisecond)
	if !browserState.RunAnimationFrame(frameTime) {
		t.Fatal("RunAnimationFrame() did not deliver WebGo callback")
	}
	box, ok := page.Document.GetElementByID("box")
	if !ok {
		t.Fatal("box element was not found")
	}
	atFrame, _ := page.AnimatedStyles(frameTime).For(box)
	if atFrame.Opacity != 0 {
		t.Fatalf("opacity at frame = %v, want transition start value 0", atFrame.Opacity)
	}
	midpoint, _ := page.AnimatedStyles(frameTime.Add(500 * time.Millisecond)).For(box)
	if midpoint.Opacity != 0.5 {
		t.Fatalf("opacity at shared timestamp midpoint = %v, want 0.5", midpoint.Opacity)
	}
	if err := browserState.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserCloseReleasesPageClientStorageAndRuntimeReferences(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/page")
	runtime := &runtimeStub{}
	browserState := NewWithRuntimeFactoryAndStorage(stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<script type="text/go">package main; func main() {}</script><p>Page</p>`),
	}}, func() runtimemodel.Runtime { return runtime }, storagecore.NewManager())
	browserState.SetOnMutation(func() {})
	if _, err := browserState.Navigate(context.Background(), pageURL.String()); err != nil {
		t.Fatal(err)
	}
	if err := browserState.Close(); err != nil {
		t.Fatal(err)
	}
	if browserState.page != nil || browserState.client != nil || browserState.storage != nil || browserState.activeRuntime != nil ||
		browserState.runtimeFactory != nil || browserState.engineFactory != nil || browserState.onMutation != nil || len(browserState.history.entries) != 0 {
		t.Fatal("Browser Close retained Page-owned references")
	}
	if runtime.stopCalls.Load() != 1 {
		t.Fatalf("Runtime Stop calls = %d, want 1", runtime.stopCalls.Load())
	}
	if err := browserState.Close(); err != nil {
		t.Fatal(err)
	}
}

func (runtime *runtimeStub) Load(_ context.Context, scripts []runtimemodel.Script, environment runtimemodel.Environment) error {
	runtime.loadCalls.Add(1)
	runtime.scripts = append([]runtimemodel.Script(nil), scripts...)
	runtime.environment = environment
	return runtime.loadErr
}

func TestEngineSelectionReloadsSelectedScriptsAndRemainsTabScoped(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/dual.html")
	secondURL := mustParseURL(t, "http://localhost/second.html")
	body := []byte(`<main id="app"></main>
<script type="text/go">package main; func main() {}</script>
<script>globalThis.started = true</script>`)
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String():   {URL: pageURL, StatusCode: 200, ContentType: "text/html", Body: body},
		secondURL.String(): {URL: secondURL, StatusCode: 200, ContentType: "text/html", Body: body},
	}}
	created := make(map[runtimemodel.Engine][]*runtimeStub)
	factory := func(engine runtimemodel.Engine) runtimemodel.Runtime {
		runtime := &runtimeStub{}
		created[engine] = append(created[engine], runtime)
		return runtime
	}
	browserState := NewWithEngineFactory(loader, factory)
	otherTab := NewWithEngineFactory(loader, factory)
	if browserState.Engine() != runtimemodel.EngineGo || otherTab.Engine() != runtimemodel.EngineGo {
		t.Fatal("new Browser did not default to Go")
	}
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if page.Engine != runtimemodel.EngineGo || len(page.Scripts) != 1 || page.Scripts[0].Engine != runtimemodel.EngineGo {
		t.Fatalf("default Page engine=%q scripts=%#v", page.Engine, page.Scripts)
	}
	firstGo := created[runtimemodel.EngineGo][0]
	page, err = browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	if firstGo.stopCalls.Load() != 1 {
		t.Fatalf("old Go Runtime Stop calls = %d, want 1", firstGo.stopCalls.Load())
	}
	if page.Engine != runtimemodel.EngineJavaScript || len(page.Scripts) != 1 ||
		page.Scripts[0].Engine != runtimemodel.EngineJavaScript || !strings.Contains(page.Scripts[0].Source, "globalThis") {
		t.Fatalf("switched Page engine=%q scripts=%#v", page.Engine, page.Scripts)
	}
	if browserState.Engine() != runtimemodel.EngineJavaScript || otherTab.Engine() != runtimemodel.EngineGo {
		t.Fatal("Engine selection leaked to another Browser tab")
	}
	if got := len(browserState.history.entries); got != 1 {
		t.Fatalf("Engine reload history entries = %d, want 1", got)
	}
	if _, err := browserState.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if browserState.Page().Engine != runtimemodel.EngineJavaScript || len(created[runtimemodel.EngineJavaScript]) != 2 {
		t.Fatal("normal Reload did not preserve JavaScript Engine")
	}
	if _, err := browserState.Navigate(context.Background(), secondURL.String()); err != nil {
		t.Fatal(err)
	}
	if page, err := browserState.Back(context.Background()); err != nil || page.Engine != runtimemodel.EngineJavaScript || page.URL.String() != pageURL.String() {
		t.Fatalf("Back() = (page=%v, error=%v), want JavaScript first Page", page, err)
	}
	if page, err := browserState.Forward(context.Background()); err != nil || page.Engine != runtimemodel.EngineJavaScript || page.URL.String() != secondURL.String() {
		t.Fatalf("Forward() = (page=%v, error=%v), want JavaScript second Page", page, err)
	}
}

func TestEngineSelectionFetchesOnlySelectedExternalSource(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/dual-external.html")
	goURL := mustParseURL(t, "http://localhost/app.go")
	javaScriptURL := mustParseURL(t, "http://localhost/app.js")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<script type="text/go" src="/app.go"></script><script src="/app.js"></script>`),
		},
		goURL.String():         {URL: goURL, StatusCode: 200, ContentType: "text/go", Body: []byte(`package main; func main() {}`)},
		javaScriptURL.String(): {URL: javaScriptURL, StatusCode: 200, ContentType: "text/javascript", Body: []byte(`globalThis.loaded = true`)},
	}}
	browserState := NewWithEngineFactory(loader, func(runtimemodel.Engine) runtimemodel.Runtime { return &runtimeStub{} })
	if _, err := browserState.Navigate(context.Background(), pageURL.String()); err != nil {
		t.Fatal(err)
	}
	if got, want := loader.requested, []string{pageURL.String(), goURL.String()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Go Engine requests = %v, want %v", got, want)
	}
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	if got, want := loader.requested, []string{pageURL.String(), goURL.String(), pageURL.String(), javaScriptURL.String()}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Engine switch requests = %v, want %v", got, want)
	}
}

func TestEngineCanBeSelectedBeforeFirstNavigationAndRejectsUnknown(t *testing.T) {
	browserState := NewWithEngineFactory(nil, func(runtimemodel.Engine) runtimemodel.Runtime { return &runtimeStub{} })
	page, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript)
	if err != nil || page != nil || browserState.Engine() != runtimemodel.EngineJavaScript {
		t.Fatalf("SetEngine before navigation = (%v, %v), engine=%q", page, err, browserState.Engine())
	}
	if _, err := browserState.SetEngine(context.Background(), "unknown"); !errors.Is(err, ErrInvalidEngine) {
		t.Fatalf("unknown SetEngine error = %v, want ErrInvalidEngine", err)
	}
}

func TestFailedEngineReloadNeverReusesStoppedRuntime(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/failure.html")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<script type="text/go">package main; func main() {}</script><script>started = true</script>`),
		},
	}}
	goRuntime := &runtimeStub{}
	createdJavaScript := 0
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime {
		if engine == runtimemodel.EngineGo {
			return goRuntime
		}
		createdJavaScript++
		return &runtimeStub{}
	})
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	delete(loader.responses, pageURL.String())
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err == nil {
		t.Fatal("SetEngine reload error = nil, want document failure")
	}
	if goRuntime.stopCalls.Load() != 1 || browserState.activeRuntime != nil || page.RuntimeStarted {
		t.Fatalf("failed switch retained Runtime: stop=%d active=%T started=%t", goRuntime.stopCalls.Load(), browserState.activeRuntime, page.RuntimeStarted)
	}
	if browserState.Engine() != runtimemodel.EngineJavaScript || createdJavaScript != 0 {
		t.Fatalf("failed switch engine=%q JavaScript runtimes=%d", browserState.Engine(), createdJavaScript)
	}
	if selected, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil || selected != page || goRuntime.stopCalls.Load() != 1 {
		t.Fatalf("same Engine no-op = (%v, %v), stop calls=%d", selected, err, goRuntime.stopCalls.Load())
	}
}

func (runtime *runtimeStub) Start(context.Context) error {
	runtime.startCalls.Add(1)
	if runtime.mutateOnStart && runtime.environment.OnMutation != nil {
		runtime.environment.OnMutation()
	}
	if runtime.navigateOnStart != "" && runtime.environment.Navigate != nil {
		target, err := url.Parse(runtime.navigateOnStart)
		if err != nil {
			return err
		}
		if err := runtime.environment.Navigate(target); err != nil {
			return err
		}
	}
	return runtime.startErr
}

func (runtime *runtimeStub) Stop() error {
	runtime.stopCalls.Add(1)
	return nil
}

func (runtime *runtimeStub) DispatchPopState(state string) {
	runtime.popStates = append(runtime.popStates, state)
	runtime.navigationEvents = append(runtime.navigationEvents, "popstate")
}

func (runtime *runtimeStub) DispatchHashChange(oldURL, newURL string) {
	runtime.hashChanges = append(runtime.hashChanges, [2]string{oldURL, newURL})
	runtime.navigationEvents = append(runtime.navigationEvents, "hashchange")
}

func TestWebGoNavigationUsesBrowserLifecycleAfterPageActivation(t *testing.T) {
	firstURL := mustParseURL(t, "http://localhost/app/index.html")
	secondURL := mustParseURL(t, "http://localhost/next")
	loader := &routeLoader{responses: map[string]*network.Response{
		firstURL.String():  {URL: firstURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<script type="text/go">package main; func main() {}</script><p>First</p>`)},
		secondURL.String(): {URL: secondURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<p>Second</p>`)},
	}}
	runtime := &runtimeStub{navigateOnStart: secondURL.String()}
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return runtime })
	if _, err := browser.Navigate(context.Background(), firstURL.String()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for (browser.Page().URL.String() != secondURL.String() || runtime.stopCalls.Load() != 1) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := browser.Page().URL.String(); got != secondURL.String() {
		t.Fatalf("active URL = %q, want %q", got, secondURL)
	}
	if got, want := len(browser.history.entries), 2; got != want {
		t.Fatalf("history entries = %d, want %d", got, want)
	}
	if runtime.stopCalls.Load() != 1 {
		t.Fatalf("previous Runtime Stop() calls = %d, want 1", runtime.stopCalls.Load())
	}
}

func TestWebGoPushStateAddsSameDocumentHistoryEntry(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/notes")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {URL: pageURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<script type="text/go">package main; func main() {}</script>`)},
	}}
	runtime := &runtimeStub{}
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return runtime })
	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	target := mustParseURL(t, "http://localhost/notes/7?mode=edit")
	if err := runtime.environment.HistoryPush(`{"note":7}`, target); err != nil {
		t.Fatalf("HistoryPush() error = %v", err)
	}
	if browser.Page() != page || page.URL.String() != target.String() {
		t.Fatalf("same-document Page = %p URL = %v", browser.Page(), page.URL)
	}
	if got := len(loader.requested); got != 1 {
		t.Fatalf("network requests = %d, want 1", got)
	}
	if got, want := len(browser.history.entries), 2; got != want {
		t.Fatalf("history entries = %d, want %d", got, want)
	}
	entry := browser.history.entries[browser.history.index]
	if entry.State != `{"note":7}` || !entry.SameDocument || entry.PageID != page.HistoryID {
		t.Fatalf("history entry = %#v", entry)
	}
	if len(runtime.popStates) != 0 || len(runtime.hashChanges) != 0 {
		t.Fatalf("PushState dispatched events: pop=%v hash=%v", runtime.popStates, runtime.hashChanges)
	}
	if _, err := browser.Back(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Forward(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runtime.popStates) != 2 || runtime.popStates[0] != "" || runtime.popStates[1] != `{"note":7}` {
		t.Fatalf("traversal popstate events = %v", runtime.popStates)
	}
	if len(runtime.hashChanges) != 0 {
		t.Fatalf("History API traversal hashchange events = %v, want none", runtime.hashChanges)
	}
}

func TestWebGoReplaceStateDoesNotAddHistoryEntry(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/notes")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {URL: pageURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<script type="text/go">package main; func main() {}</script>`)},
	}}
	runtime := &runtimeStub{}
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return runtime })
	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	target := mustParseURL(t, "http://localhost/notes?filter=open")
	if err := runtime.environment.HistoryReplace(`{"filter":"open"}`, target); err != nil {
		t.Fatalf("HistoryReplace() error = %v", err)
	}
	if browser.Page() != page || page.URL.String() != target.String() {
		t.Fatalf("active Page or URL changed unexpectedly: %p %v", browser.Page(), page.URL)
	}
	if got, want := len(browser.history.entries), 1; got != want {
		t.Fatalf("history entries = %d, want %d", got, want)
	}
	entry := browser.history.entries[0]
	if entry.State != `{"filter":"open"}` || !entry.SameDocument {
		t.Fatalf("history entry = %#v", entry)
	}
	if got := len(loader.requested); got != 1 {
		t.Fatalf("network requests = %d, want 1", got)
	}
	if len(runtime.popStates) != 0 || len(runtime.hashChanges) != 0 {
		t.Fatalf("ReplaceState dispatched events: pop=%v hash=%v", runtime.popStates, runtime.hashChanges)
	}
}

func TestBrowserRejectsUnvalidatedHistoryInputWithoutMutation(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/safe")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {URL: pageURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<p>Safe</p>`)},
	}}
	browser := New(loader)
	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	credentialURL := mustParseURL(t, "http://alice:super-secret@localhost/private")
	for _, test := range []struct {
		state  string
		target *url.URL
	}{
		{state: `{`, target: pageURL},
		{state: `"` + strings.Repeat("x", maxHistoryStateBytes) + `"`, target: pageURL},
		{state: `null`, target: credentialURL},
	} {
		err := browser.pushHistoryState(page, test.target, test.state)
		if err == nil {
			t.Fatal("pushHistoryState() accepted unsafe input")
		}
		if strings.Contains(err.Error(), "super-secret") || strings.Contains(err.Error(), test.state) {
			t.Fatalf("error exposed unsafe input: %q", err)
		}
	}
	if len(browser.history.entries) != 1 || page.URL.String() != pageURL.String() {
		t.Fatalf("rejected input mutated History or URL: entries=%d URL=%v", len(browser.history.entries), page.URL)
	}
}

func TestWebGoHistoryTraversalUsesCrossDocumentLifecycle(t *testing.T) {
	firstURL := mustParseURL(t, "http://localhost/first")
	secondURL := mustParseURL(t, "http://localhost/second")
	loader := &routeLoader{responses: map[string]*network.Response{
		firstURL.String():  {URL: firstURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<p>First</p>`)},
		secondURL.String(): {URL: secondURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<script type="text/go">package main; func main() {}</script><p>Second</p>`)},
	}}
	runtime := &runtimeStub{}
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return runtime })
	if _, err := browser.Navigate(context.Background(), firstURL.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Navigate(context.Background(), secondURL.String()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.environment.HistoryTraverse(-1); err != nil {
		t.Fatalf("HistoryTraverse() error = %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for (browser.Page().URL.String() != firstURL.String() || runtime.stopCalls.Load() != 1) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := browser.Page().URL.String(); got != firstURL.String() {
		t.Fatalf("active URL = %q, want %q", got, firstURL)
	}
	if got, want := len(browser.history.entries), 2; got != want {
		t.Fatalf("history entries = %d, want %d", got, want)
	}
	if browser.history.index != 0 || runtime.stopCalls.Load() != 1 {
		t.Fatalf("history index = %d, Runtime Stop calls = %d", browser.history.index, runtime.stopCalls.Load())
	}
}

func TestCrossDocumentTraversalDispatchesPopStateToNewRuntime(t *testing.T) {
	firstURL := mustParseURL(t, "http://localhost/first-state")
	secondURL := mustParseURL(t, "http://localhost/second-state")
	script := `<script type="text/go">package main; func main() {}</script>`
	loader := &routeLoader{responses: map[string]*network.Response{
		firstURL.String():  {URL: firstURL, StatusCode: 200, ContentType: "text/html", Body: []byte(script)},
		secondURL.String(): {URL: secondURL, StatusCode: 200, ContentType: "text/html", Body: []byte(script)},
	}}
	var runtimes []*runtimeStub
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime {
		runtime := &runtimeStub{}
		runtimes = append(runtimes, runtime)
		return runtime
	})
	first, err := browser.Navigate(context.Background(), firstURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := browser.replaceHistoryState(first, firstURL, `{"document":"first"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Navigate(context.Background(), secondURL.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Back(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runtimes) != 3 {
		t.Fatalf("Runtime count = %d, want 3", len(runtimes))
	}
	if got := runtimes[2].popStates; len(got) != 1 || got[0] != `{"document":"first"}` {
		t.Fatalf("new Runtime popstate events = %v", got)
	}
}

func TestNavigationSwitchesStorageByOrigin(t *testing.T) {
	firstURL := mustParseURL(t, "http://localhost/storage-a")
	sameOriginURL := mustParseURL(t, "http://localhost/storage-b?view=2")
	otherOriginURL := mustParseURL(t, "http://127.0.0.1/storage-c")
	script := `<script type="text/go">package main; func main() {}</script>`
	loader := &routeLoader{responses: map[string]*network.Response{
		firstURL.String():       {URL: firstURL, StatusCode: 200, ContentType: "text/html", Body: []byte(script)},
		sameOriginURL.String():  {URL: sameOriginURL, StatusCode: 200, ContentType: "text/html", Body: []byte(script)},
		otherOriginURL.String(): {URL: otherOriginURL, StatusCode: 200, ContentType: "text/html", Body: []byte(script)},
	}}
	var runtimes []*runtimeStub
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime {
		runtime := &runtimeStub{}
		runtimes = append(runtimes, runtime)
		return runtime
	})
	if _, err := browser.Navigate(context.Background(), firstURL.String()); err != nil {
		t.Fatal(err)
	}
	runtimes[0].environment.LocalStorage.Set("shared", "yes")
	if _, err := browser.Navigate(context.Background(), sameOriginURL.String()); err != nil {
		t.Fatal(err)
	}
	if got, found := runtimes[1].environment.LocalStorage.Get("shared"); !found || got != "yes" {
		t.Fatalf("same-Origin Local Storage = (%q, %v)", got, found)
	}
	if _, err := browser.Navigate(context.Background(), otherOriginURL.String()); err != nil {
		t.Fatal(err)
	}
	if got, found := runtimes[2].environment.LocalStorage.Get("shared"); found || got != "" {
		t.Fatalf("cross-Origin Local Storage leaked = (%q, %v)", got, found)
	}
}

func TestSessionStorageSurvivesSameDocumentNavigationAndReload(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/session-storage")
	script := `<script type="text/go">package main; func main() {}</script>`
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {URL: pageURL, StatusCode: 200, ContentType: "text/html", Body: []byte(script)},
	}}
	var runtimes []*runtimeStub
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime {
		runtime := &runtimeStub{}
		runtimes = append(runtimes, runtime)
		return runtime
	})
	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimes[0].environment.SessionStorage.Set("draft", "kept"); err != nil {
		t.Fatal(err)
	}
	if err := runtimes[0].environment.HistoryPush(`{"route":2}`, pageURL); err != nil {
		t.Fatal(err)
	}
	if browser.Page() != page || len(runtimes) != 1 {
		t.Fatal("same-document Navigation replaced Page or Runtime")
	}
	if got, found := runtimes[0].environment.SessionStorage.Get("draft"); !found || got != "kept" {
		t.Fatalf("same-document Session Storage = (%q, %v)", got, found)
	}

	reloaded, err := browser.Reload(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reloaded == page || len(runtimes) != 2 {
		t.Fatal("reload did not rebuild Page and Runtime")
	}
	if got, found := runtimes[1].environment.SessionStorage.Get("draft"); !found || got != "kept" {
		t.Fatalf("reloaded Session Storage = (%q, %v)", got, found)
	}
	if runtimes[0].environment.SessionStorage != runtimes[1].environment.SessionStorage {
		t.Fatal("reload switched Session Storage Area")
	}
}

func TestNewBrowserSessionDoesNotInheritSessionStorage(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/new-session")
	script := `<script type="text/go">package main; func main() {}</script>`
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {URL: pageURL, StatusCode: 200, ContentType: "text/html", Body: []byte(script)},
	}}
	root := t.TempDir()
	firstManager, err := storagecore.NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	firstRuntime := &runtimeStub{}
	firstBrowser := NewWithRuntimeFactoryAndStorage(loader, func() runtimemodel.Runtime { return firstRuntime }, firstManager)
	if _, err := firstBrowser.Navigate(context.Background(), pageURL.String()); err != nil {
		t.Fatal(err)
	}
	if err := firstRuntime.environment.LocalStorage.Set("local", "persisted"); err != nil {
		t.Fatal(err)
	}
	if err := firstRuntime.environment.SessionStorage.Set("session", "temporary"); err != nil {
		t.Fatal(err)
	}
	if err := firstBrowser.Close(); err != nil {
		t.Fatal(err)
	}

	secondManager, err := storagecore.NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	secondRuntime := &runtimeStub{}
	secondBrowser := NewWithRuntimeFactoryAndStorage(loader, func() runtimemodel.Runtime { return secondRuntime }, secondManager)
	if _, err := secondBrowser.Navigate(context.Background(), pageURL.String()); err != nil {
		t.Fatal(err)
	}
	if got, found := secondRuntime.environment.LocalStorage.Get("local"); !found || got != "persisted" {
		t.Fatalf("new Browser Local Storage = (%q, %v)", got, found)
	}
	if got, found := secondRuntime.environment.SessionStorage.Get("session"); found || got != "" {
		t.Fatalf("new Browser inherited Session Storage = (%q, %v)", got, found)
	}
}

func TestNavigateStartsRuntimeForTrustedOrigin(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/index.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<style>#running { animation: 1s linear infinite pulse; }</style>
<div id="running"></div><script type="text/go">package main
func main() {}</script>`),
	}}
	runtime := &runtimeStub{}
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return runtime })

	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if !page.RuntimeStarted || page.RuntimeError != "" {
		t.Fatalf("runtime state = started:%v error:%q", page.RuntimeStarted, page.RuntimeError)
	}
	if runtime.loadCalls.Load() != 1 || runtime.startCalls.Load() != 1 {
		t.Fatalf("runtime calls = load:%d start:%d, want 1 each", runtime.loadCalls.Load(), runtime.startCalls.Load())
	}
	running, ok := page.Document.GetElementByID("running")
	if !ok || page.Animations.Count(running.ID) != 1 {
		t.Fatal("running CSS animation was not registered")
	}
	if err := browser.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if runtime.stopCalls.Load() != 1 {
		t.Fatalf("Stop() calls = %d, want 1", runtime.stopCalls.Load())
	}
	if page.Animations.Count(running.ID) != 0 {
		t.Fatalf("animation count after Runtime stop = %d, want zero", page.Animations.Count(running.ID))
	}
}

func TestNavigateBlocksRuntimeForUntrustedOrigin(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/index.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<script type="text/go">package main
func main() {}</script>`),
	}}
	created := false
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime {
		created = true
		return &runtimeStub{}
	})

	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if created || page.RuntimeStarted || !strings.Contains(page.RuntimeError, "untrusted origin") {
		t.Fatalf("runtime state = created:%v started:%v error:%q", created, page.RuntimeStarted, page.RuntimeError)
	}
}

func TestRuntimeStartErrorDoesNotPreventPageActivation(t *testing.T) {
	pageURL := mustParseURL(t, "http://127.0.0.1/index.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<script type="text/go">package main
func main() {}</script>`),
	}}
	runtime := &runtimeStub{startErr: errors.New("compile failed")}
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return runtime })

	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if browser.Page() != page || page.RuntimeStarted || !strings.Contains(page.RuntimeError, "compile failed") {
		t.Fatalf("page runtime state = active:%v started:%v error:%q", browser.Page() == page, page.RuntimeStarted, page.RuntimeError)
	}
	if runtime.stopCalls.Load() != 1 {
		t.Fatalf("Stop() calls = %d, want failed runtime cleanup", runtime.stopCalls.Load())
	}
	records := page.DevTools.Console()
	if len(records) != 1 || records[0].Level != devtools.ConsoleError || records[0].Source != "runtime" || !strings.Contains(records[0].Message, "compile failed") {
		t.Fatalf("runtime console records = %+v", records)
	}
}

func TestConsoleCallbackCannotWriteAfterPageClose(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/index.html")
	runtime := &runtimeStub{}
	browserState := NewWithRuntimeFactory(stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<script type="text/go">package main; func main() {}</script>`),
	}}, func() runtimemodel.Runtime { return runtime })
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	runtime.environment.ConsoleRecord("warn", "before close")
	if got := len(page.DevTools.Console()); got != 1 {
		t.Fatalf("records before close = %d, want 1", got)
	}
	if err := browserState.Close(); err != nil {
		t.Fatal(err)
	}
	runtime.environment.ConsoleRecord("error", "after close")
	if got := len(page.DevTools.Console()); got != 0 {
		t.Fatalf("records after close callback = %d, want 0", got)
	}
}

func TestRuntimeMutationNotifiesBrowser(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/index.html")
	loader := stubLoader{response: &network.Response{
		URL: pageURL, StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<script type="text/go">package main
func main() {}</script>`),
	}}
	runtime := &runtimeStub{mutateOnStart: true}
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return runtime })
	mutations := 0
	browser.SetOnMutation(func() { mutations++ })

	if _, err := browser.Navigate(context.Background(), pageURL.String()); err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if mutations != 1 {
		t.Fatalf("mutation count = %d, want 1", mutations)
	}
}

func TestNavigationAndReloadStopPreviousPageRuntime(t *testing.T) {
	firstURL := mustParseURL(t, "http://localhost/first.html")
	secondURL := mustParseURL(t, "http://localhost/second.html")
	body := []byte(`<script type="text/go">package main
func main() {}</script>`)
	loader := &routeLoader{responses: map[string]*network.Response{
		firstURL.String():  {URL: firstURL, StatusCode: 200, ContentType: "text/html", Body: body},
		secondURL.String(): {URL: secondURL, StatusCode: 200, ContentType: "text/html", Body: body},
	}}
	var runtimes []*runtimeStub
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime {
		runtime := &runtimeStub{}
		runtimes = append(runtimes, runtime)
		return runtime
	})
	if _, err := browser.Navigate(context.Background(), firstURL.String()); err != nil {
		t.Fatalf("first Navigate() error = %v", err)
	}
	if _, err := browser.Navigate(context.Background(), secondURL.String()); err != nil {
		t.Fatalf("second Navigate() error = %v", err)
	}
	if len(runtimes) != 2 || runtimes[0].stopCalls.Load() != 1 {
		t.Fatalf("after navigation runtimes = %d first stops = %d", len(runtimes), runtimes[0].stopCalls.Load())
	}
	if _, err := browser.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if len(runtimes) != 3 || runtimes[1].stopCalls.Load() != 1 {
		t.Fatalf("after reload runtimes = %d second stops = %d", len(runtimes), runtimes[1].stopCalls.Load())
	}
}

func TestDispatchClickUsesActivePageDispatcher(t *testing.T) {
	browser := New(nil)
	dispatcher := events.NewDispatcher()
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Events = dispatcher
	browser.SetPage(page)
	called := false
	dispatcher.AddEventListener(9, events.Click, func(event events.Event) {
		called = event.X == 10 && event.Y == 20
	})

	if !browser.DispatchClick(9, 10, 20) || !called {
		t.Fatal("DispatchClick() did not dispatch to the active page")
	}
	if browser.DispatchClick(10, 10, 20) {
		t.Fatal("DispatchClick() handled a node without listeners")
	}
}
