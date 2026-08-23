package yaegi

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	dommodel "github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/forms"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
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

func TestPortableFSNormalizesWindowsSeparators(t *testing.T) {
	filesystem := portableFS{FS: fstest.MapFS{
		"src/main/page/main.go": {Data: []byte("package main")},
	}}
	file, err := filesystem.Open(`src\main\page\main.go`)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if got, want := string(content), "package main"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
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

func TestRuntimeExposesURLSearchParams(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/url"
var Encoded string
var First string
func main() {
	params, err := url.Parse("tag=go&tag=web+api&empty=")
	if err != nil { return }
	_ = params.Set("tag", "growse")
	_ = params.Append("page", "1")
	First, _ = params.Get("tag")
	Encoded, _ = params.Encode()
}`}}
	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	symbols := runtime.interpreter.Symbols("page")["page"]
	if got, want := symbols["First"].String(), "growse"; got != want {
		t.Fatalf("First = %q, want %q", got, want)
	}
	if got, want := symbols["Encoded"].String(), "tag=growse&empty=&page=1"; got != want {
		t.Fatalf("Encoded = %q, want %q", got, want)
	}
}

func TestRuntimeExposesFormData(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/form"
var Encoded string
func main() {
	data := form.New()
	_ = data.Append("name", "Growse")
	_ = data.Append("tag", "web api")
	Encoded, _ = data.Encode()
}`}}
	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := runtime.interpreter.Symbols("page")["page"]["Encoded"].String(), "name=Growse&tag=web+api"; got != want {
		t.Fatalf("Encoded = %q, want %q", got, want)
	}
}

func TestRuntimeExposesLocalAndSessionStorage(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/storage"
var LocalValue string
var SessionValue string
var Failure string
func main() {
	local := storage.Local()
	session := storage.Session()
	if err := local.Set("mode", "local"); err != nil { Failure = err.Error(); return }
	if err := session.Set("mode", "session"); err != nil { Failure = err.Error(); return }
	LocalValue, _, _ = local.Get("mode")
	SessionValue, _, _ = session.Get("mode")
	_ = local.Remove("mode")
	_ = session.Clear()
}`}}
	environment := runtimemodel.Environment{LocalStorage: storagecore.NewArea(), SessionStorage: storagecore.NewArea()}
	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	symbols := runtime.interpreter.Symbols("page")["page"]
	if got := symbols["Failure"].String(); got != "" {
		t.Fatalf("Failure = %q", got)
	}
	if got := symbols["LocalValue"].String(); got != "local" {
		t.Fatalf("LocalValue = %q", got)
	}
	if got := symbols["SessionValue"].String(); got != "session" {
		t.Fatalf("SessionValue = %q", got)
	}
}

func TestRuntimeExposesWebGoTimeoutAndInterval(t *testing.T) {
	runtime := New()
	mutated := make(chan struct{}, 4)
	document := dommodel.NewDocument()
	result := document.CreateElement("p", map[string]string{"id": "result"})
	if err := document.AppendChild(document.Root, result); err != nil {
		t.Fatal(err)
	}
	scripts := []runtimemodel.Script{{Source: `package main
import (
	"growse/dom"
	"growse/scheduler"
)
var TimeoutID scheduler.TimerID
var IntervalID scheduler.TimerID
func main() {
	TimeoutID, _ = scheduler.SetTimeout(0, func() {
		dom.GetElementByID("result").SetText("timeout")
	})
	IntervalID, _ = scheduler.SetInterval(scheduler.Millisecond, func() {
		dom.GetElementByID("result").SetText("interval")
	})
}`}}
	environment := runtimemodel.Environment{
		Document: document,
		Events:   events.NewDispatcher(),
		OnMutation: func() {
			mutated <- struct{}{}
		},
	}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-mutated:
	case <-time.After(time.Second):
		t.Fatal("Scheduler callback was not delivered through the page queue")
	}
	symbols := runtime.interpreter.Symbols("page")["page"]
	if symbols["TimeoutID"].Uint() == 0 || symbols["IntervalID"].Uint() == 0 {
		t.Fatalf("timer IDs = timeout:%d interval:%d, want non-zero", symbols["TimeoutID"].Uint(), symbols["IntervalID"].Uint())
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestRuntimeExposesWebGoTimerCancellation(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/scheduler"
var Cleared bool
func main() {
	id, _ := scheduler.SetTimeout(scheduler.Second, func() {})
	Cleared = scheduler.ClearTimer(id)
}`}}
	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !runtime.interpreter.Symbols("page")["page"]["Cleared"].Bool() {
		t.Fatal("WebGo ClearTimer() did not cancel the active timeout")
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestRuntimeExposesAnimationFrameRegistrationAndCancellation(t *testing.T) {
	runtime := New()
	document := dommodel.NewDocument()
	result := document.CreateElement("p", map[string]string{"id": "result"})
	if err := document.AppendChild(document.Root, result); err != nil {
		t.Fatal(err)
	}
	frameRequests := 0
	mutated := make(chan struct{}, 1)
	scripts := []runtimemodel.Script{{Source: `package main
import (
	"growse/dom"
	"growse/scheduler"
)
var FrameID scheduler.FrameID
var Canceled bool
var LastTimestamp scheduler.Timestamp
func main() {
	canceledID, _ := scheduler.RequestAnimationFrame(func(scheduler.Timestamp) {
		dom.GetElementByID("result").SetText("canceled")
	})
	Canceled = scheduler.CancelAnimationFrame(canceledID)
	FrameID, _ = scheduler.RequestAnimationFrame(func(timestamp scheduler.Timestamp) {
		LastTimestamp = timestamp
		dom.GetElementByID("result").SetText("frame")
	})
}`}}
	environment := runtimemodel.Environment{
		Document: document,
		Events:   events.NewDispatcher(),
		RequestFrame: func() {
			frameRequests++
		},
		OnMutation: func() {
			mutated <- struct{}{}
		},
	}
	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if frameRequests != 2 {
		t.Fatalf("frame requests = %d, want 2", frameRequests)
	}
	if !runtime.RunAnimationFrame(time.Now()) {
		t.Fatal("RunAnimationFrame() did not deliver the active callback")
	}
	select {
	case <-mutated:
	default:
		t.Fatal("frame callback did not mutate the DOM")
	}
	symbols := runtime.interpreter.Symbols("page")["page"]
	if symbols["FrameID"].Uint() == 0 || !symbols["Canceled"].Bool() {
		t.Fatalf("frame state = id:%d canceled:%v", symbols["FrameID"].Uint(), symbols["Canceled"].Bool())
	}
	if got := result.TextContent(); got != "frame" {
		t.Fatalf("result text = %q, want frame", got)
	}
	if runtime.HasAnimationFrameCallbacks() {
		t.Fatal("one-shot frame callback remained active")
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestRuntimeExposesCurrentNavigationLocation(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/navigation"
var Href string
var Origin string
var Path string
var Query string
var Fragment string
func main() {
	current := navigation.Current()
	Href = current.Href
	Origin = current.Origin
	Path = current.Path
	Query = current.Query
	Fragment = current.Fragment
}`}}
	documentURL, err := url.Parse("https://example.test:8443/notes/today?filter=open#second")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{BaseURL: documentURL}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	symbols := runtime.interpreter.Symbols("page")["page"]
	for name, want := range map[string]string{
		"Href":   "https://example.test:8443/notes/today?filter=open#second",
		"Origin": "https://example.test:8443", "Path": "/notes/today",
		"Query": "filter=open", "Fragment": "second",
	} {
		if got := symbols[name].String(); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestRuntimeResolvesAndRequestsWebGoNavigation(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/navigation"
var Resolved string
var Failure string
func main() {
	location, err := navigation.Resolve("../next?from=webgo")
	if err != nil { Failure = err.Error(); return }
	Resolved = location.Href
	if err := navigation.Navigate("../next?from=webgo"); err != nil { Failure = err.Error() }
}`}}
	baseURL, _ := url.Parse("http://localhost/app/index.html")
	var requested *url.URL
	environment := runtimemodel.Environment{BaseURL: baseURL, Navigate: func(target *url.URL) error {
		requested = target
		return nil
	}}
	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	symbols := runtime.interpreter.Symbols("page")["page"]
	if got := symbols["Failure"].String(); got != "" {
		t.Fatalf("Failure = %q", got)
	}
	const want = "http://localhost/next?from=webgo"
	if got := symbols["Resolved"].String(); got != want {
		t.Fatalf("Resolved = %q, want %q", got, want)
	}
	if requested == nil || requested.String() != want {
		t.Fatalf("requested URL = %v, want %q", requested, want)
	}
}

func TestRuntimePushesJSONHistoryState(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/navigation"
var Failure string
func main() {
	if err := navigation.PushState("{\"view\":\"detail\"}", "/notes/9"); err != nil { Failure = err.Error() }
}`}}
	baseURL, _ := url.Parse("http://localhost/notes")
	var state string
	var target *url.URL
	environment := runtimemodel.Environment{
		BaseURL: baseURL,
		HistoryPush: func(gotState string, gotTarget *url.URL) error {
			state, target = gotState, gotTarget
			return nil
		},
	}
	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := runtime.interpreter.Symbols("page")["page"]["Failure"].String(); got != "" {
		t.Fatalf("Failure = %q", got)
	}
	if state != `{"view":"detail"}` || target == nil || target.String() != "http://localhost/notes/9" {
		t.Fatalf("HistoryPush = (%q, %v)", state, target)
	}
}

func TestRuntimeExposesHistoryTraversalAndSnapshot(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/navigation"
var Length int
var State string
var Failure string
func main() {
	Length = navigation.HistoryLength()
	State = navigation.HistoryState()
	if err := navigation.Go(-2); err != nil { Failure = err.Error() }
}`}}
	var delta int
	environment := runtimemodel.Environment{
		HistoryTraverse: func(got int) error { delta = got; return nil },
		HistoryInfo:     func() (int, string) { return 3, `{"route":"notes"}` },
	}
	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	symbols := runtime.interpreter.Symbols("page")["page"]
	if got := int(symbols["Length"].Int()); got != 3 {
		t.Fatalf("Length = %d, want 3", got)
	}
	if got := symbols["State"].String(); got != `{"route":"notes"}` {
		t.Fatalf("State = %q", got)
	}
	if got := symbols["Failure"].String(); got != "" || delta != -2 {
		t.Fatalf("traversal = delta:%d failure:%q", delta, got)
	}
}

func TestRuntimeDeliversNavigationEventsThroughPageQueue(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
import "growse/navigation"
var PopState string
var OldURL string
var NewURL string
func main() {
	navigation.OnPopState(func(event navigation.PopStateEvent) {
		PopState = event.State
		dom.GetElementByID("result").SetText("pop")
	})
	navigation.OnHashChange(func(event navigation.HashChangeEvent) {
		OldURL = event.OldURL
		NewURL = event.NewURL
		dom.GetElementByID("result").SetText("hash")
	})
}`}}
	document := dommodel.NewDocument()
	result := document.CreateElement("p", map[string]string{"id": "result"})
	if err := document.AppendChild(document.Root, result); err != nil {
		t.Fatal(err)
	}
	mutated := make(chan struct{}, 2)
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(),
		OnMutation: func() { mutated <- struct{}{} },
	}
	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	runtime.DispatchPopState(`{"page":2}`)
	runtime.DispatchHashChange("https://example.test/#one", "https://example.test/#two")
	for range 2 {
		select {
		case <-mutated:
		case <-time.After(time.Second):
			t.Fatal("Navigation Event callback was not delivered")
		}
	}
	symbols := runtime.interpreter.Symbols("page")["page"]
	if got := symbols["PopState"].String(); got != `{"page":2}` {
		t.Fatalf("PopState = %q", got)
	}
	if got := symbols["OldURL"].String(); got != "https://example.test/#one" {
		t.Fatalf("OldURL = %q", got)
	}
	if got := symbols["NewURL"].String(); got != "https://example.test/#two" {
		t.Fatalf("NewURL = %q", got)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeFetchSendsMethodRelativeURLHeadersAndTextBody(t *testing.T) {
	runtime := New()
	var captured *network.Request
	document := dommodel.NewDocument()
	result := document.CreateElement("p", map[string]string{"id": "result"})
	if err := document.AppendChild(document.Root, result); err != nil {
		t.Fatal(err)
	}
	mutated := make(chan struct{}, 1)
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/fetch"
import "growse/dom"
var Status int
var Failure string
func main() {
	headers := fetch.NewHeaders()
	_ = headers.Append("X-Test", "one")
	_ = headers.Append("X-Test", "two")
	fetch.Fetch(fetch.Request{
		Method: "PATCH",
		URL: "/items/7",
		Headers: headers,
		Text: "updated",
	}, func(response fetch.Response) {
		Status = response.Status
		dom.GetElementByID("result").SetText("fetched")
	}, func(message string) {
		Failure = message
	})
}`}}
	baseURL, err := url.Parse("https://example.test/app/index.html")
	if err != nil {
		t.Fatal(err)
	}
	environment := runtimemodel.Environment{
		BaseURL: baseURL, Document: document, Events: events.NewDispatcher(),
		OnMutation: func() { mutated <- struct{}{} },
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			captured = request
			return &network.Response{StatusCode: http.StatusNoContent}, nil
		},
	}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-mutated:
	case <-time.After(time.Second):
		t.Fatal("Fetch callback was not delivered through the page queue")
	}
	if captured == nil {
		t.Fatal("WebGo Fetch did not send a request")
	}
	if got, want := captured.Method, http.MethodPatch; got != want {
		t.Fatalf("method = %q, want %q", got, want)
	}
	if got, want := captured.URL.String(), "https://example.test/items/7"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
	if got, want := captured.Header.Values("X-Test"), []string{"one", "two"}; !equalStrings(got, want) {
		t.Fatalf("X-Test = %v, want %v", got, want)
	}
	if got, want := string(captured.Body), "updated"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	packageSymbols := runtime.interpreter.Symbols("page")["page"]
	if got := int(packageSymbols["Status"].Int()); got != http.StatusNoContent {
		t.Fatalf("Status = %d, want %d", got, http.StatusNoContent)
	}
	if got := packageSymbols["Failure"].String(); got != "" {
		t.Fatalf("Failure = %q, want empty", got)
	}
	if got, want := result.TextContent(), "fetched"; got != want {
		t.Fatalf("result text = %q, want %q", got, want)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestRuntimeFetchAcceptsFormDataBody(t *testing.T) {
	runtime := New()
	sent := make(chan *network.Request, 1)
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/fetch"
import "growse/form"
func main() {
	data := form.New()
	_ = data.Append("name", "Growse")
	fetch.Fetch(fetch.Request{Method: "POST", URL: "/items", FormData: data}, nil, nil)
}`}}
	baseURL, _ := url.Parse("https://example.test/page")
	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{
		BaseURL: baseURL,
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			sent <- request
			return &network.Response{}, nil
		},
	}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case request := <-sent:
		if got, want := string(request.Body), "name=Growse"; got != want {
			t.Fatalf("body = %q, want %q", got, want)
		}
		if got, want := request.Header.Get("Content-Type"), "application/x-www-form-urlencoded;charset=UTF-8"; got != want {
			t.Fatalf("Content-Type = %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("WebGo Fetch was not started")
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestRuntimeSerializesPageCallbacksInQueueOrder(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{{Source: "package main\nfunc main() {}"}}
	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	if !runtime.enqueueCallback(func() {
		close(firstStarted)
		<-releaseFirst
	}) || !runtime.enqueueCallback(func() { close(secondStarted) }) {
		t.Fatal("enqueueCallback() rejected an active page callback")
	}
	<-firstStarted
	select {
	case <-secondStarted:
		t.Fatal("second callback ran concurrently with the first callback")
	default:
	}
	close(releaseFirst)
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second callback was not delivered after the first callback")
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestRuntimeSerializesDOMEventBehindQueuedCallback(t *testing.T) {
	runtime := New()
	document := dommodel.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "button"})
	if err := document.AppendChild(document.Root, button); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
var Clicks int
func main() {
	dom.GetElementByID("button").OnClick(func() { Clicks++ })
}`}}
	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{
		Document: document,
		Events:   dispatcher,
	}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	if !runtime.enqueueCallback(func() {
		close(firstStarted)
		<-releaseFirst
	}) {
		t.Fatal("failed to enqueue blocking page callback")
	}
	<-firstStarted
	eventDone := make(chan bool, 1)
	go func() {
		eventDone <- runtime.DispatchPageEvent(func() bool {
			return dispatcher.Dispatch(events.Event{Type: events.Click, Target: button.ID})
		})
	}()
	select {
	case <-eventDone:
		t.Fatal("DOM event ran concurrently with an earlier page callback")
	default:
	}
	close(releaseFirst)
	select {
	case handled := <-eventDone:
		if !handled {
			t.Fatal("queued DOM event was not handled")
		}
	case <-time.After(time.Second):
		t.Fatal("queued DOM event was not delivered")
	}
	if got := runtime.interpreter.Symbols("page")["page"]["Clicks"].Int(); got != 1 {
		t.Fatalf("click count = %d, want 1", got)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestRuntimeStopCancelsInFlightFetch(t *testing.T) {
	runtime := New()
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/fetch"
func main() {
	fetch.Fetch(fetch.Request{URL: "/slow"}, func(fetch.Response) {}, func(string) {})
}`}}
	baseURL, err := url.Parse("https://example.test/page")
	if err != nil {
		t.Fatal(err)
	}
	environment := runtimemodel.Environment{
		BaseURL: baseURL,
		Fetch: func(ctx context.Context, _ *network.Request) (*network.Response, error) {
			close(requestStarted)
			<-ctx.Done()
			close(requestCanceled)
			return nil, ctx.Err()
		},
	}
	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("Fetch request did not start")
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("in-flight Fetch did not observe Runtime cancellation")
	}
}

func TestRuntimeStopDiscardsQueuedAndFutureCallbacks(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{{Source: "package main\nfunc main() {}"}}
	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	staleCallback := make(chan struct{}, 1)
	if !runtime.enqueueCallback(func() {
		close(firstStarted)
		<-releaseFirst
	}) || !runtime.enqueueCallback(func() { staleCallback <- struct{}{} }) {
		t.Fatal("failed to enqueue active page callbacks")
	}
	<-firstStarted
	stopped := make(chan struct{})
	go func() {
		_ = runtime.Stop()
		close(stopped)
	}()
	<-runtime.runtimeCtx.Done()
	close(releaseFirst)
	<-stopped
	select {
	case <-staleCallback:
		t.Fatal("queued callback was delivered after Runtime cancellation")
	default:
	}
	if runtime.enqueueCallback(func() { staleCallback <- struct{}{} }) {
		t.Fatal("stopped Runtime accepted an old Page callback")
	}
}

func TestRuntimeStopDiscardsSchedulerCallbacks(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/scheduler"
func main() {
	_, _ = scheduler.SetTimeout(scheduler.Second, func() {})
	_, _ = scheduler.RequestAnimationFrame(func(timestamp scheduler.Timestamp) {
		_ = timestamp
	})
}`}}
	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !runtime.HasAnimationFrameCallbacks() {
		t.Fatal("runtime did not retain the registered frame before Stop")
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if runtime.HasAnimationFrameCallbacks() || runtime.RunAnimationFrame(time.Now()) {
		t.Fatal("stopped runtime retained a frame callback")
	}
	if runtime.interpreter != nil || runtime.runtimeCtx != nil || runtime.callbackQueue != nil || runtime.callbackDone != nil ||
		runtime.fetchAPI != nil || runtime.navigationAPI != nil || runtime.schedulerAPI != nil {
		t.Fatal("stopped Runtime retained Page-owned references")
	}
}

func equalStrings(left, right []string) bool {
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
		Events:   events.NewDispatcher(),
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

func TestRuntimeExposesQuerySelector(t *testing.T) {
	document := dommodel.NewDocument()
	message := document.CreateElement("p", map[string]string{"class": "message featured"})
	if err := document.AppendChild(document.Root, message); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(message, document.CreateText("before")); err != nil {
		t.Fatal(err)
	}
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	element := dom.QuerySelector("p.featured")
	if element != nil {
		element.SetText("after")
	}
}`}}
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher()}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := message.TextContent(), "after"; got != want {
		t.Fatalf("TextContent() = %q, want %q", got, want)
	}
}

func TestRuntimeExposesCreateElement(t *testing.T) {
	document := dommodel.NewDocument()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
var Created *dom.Element
func main() { Created = dom.CreateElement("section") }`}}
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher()}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	packageSymbols := runtime.interpreter.Symbols("page")["page"]
	created, ok := packageSymbols["Created"]
	if !ok || created.IsNil() {
		t.Fatal("CreateElement() did not return a WebGo element")
	}
	if got, want := document.NodeCount(), 1; got != want {
		t.Fatalf("NodeCount() = %d, want %d", got, want)
	}
	if got, want := document.ElementCount(), 0; got != want {
		t.Fatalf("ElementCount() = %d, want %d before attachment", got, want)
	}
}

func TestRuntimeAppendsCreatedElement(t *testing.T) {
	document := dommodel.NewDocument()
	list := document.CreateElement("ul", map[string]string{"id": "list"})
	if err := document.AppendChild(document.Root, list); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	list := dom.GetElementByID("list")
	item := dom.CreateElement("li")
	item.SetText("created")
	list.AppendChild(item)
}`}}
	environment := runtimemodel.Environment{
		Document: document,
		Events:   events.NewDispatcher(),
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
	if got, want := len(list.Children), 1; got != want {
		t.Fatalf("list child count = %d, want %d", got, want)
	}
	if got, want := list.Children[0].TextContent(), "created"; got != want {
		t.Fatalf("item text = %q, want %q", got, want)
	}
	if got, want := mutations, 2; got != want {
		t.Fatalf("mutation count = %d, want %d", got, want)
	}
}

func TestRuntimeRemovesElement(t *testing.T) {
	document := dommodel.NewDocument()
	item := document.CreateElement("li", map[string]string{"id": "item"})
	if err := document.AppendChild(document.Root, item); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() { dom.GetElementByID("item").Remove() }`}}
	environment := runtimemodel.Environment{
		Document: document,
		Events:   events.NewDispatcher(),
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
	if _, ok := document.GetElementByID("item"); ok {
		t.Fatal("removed item remains in document")
	}
	if got, want := mutations, 1; got != want {
		t.Fatalf("mutation count = %d, want %d", got, want)
	}
}

func TestRuntimeGetsAndSetsAttributes(t *testing.T) {
	document := dommodel.NewDocument()
	item := document.CreateElement("li", map[string]string{"id": "item", "data-state": "before"})
	if err := document.AppendChild(document.Root, item); err != nil {
		t.Fatal(err)
	}
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	item := dom.GetElementByID("item")
	value, ok := item.GetAttribute("data-state")
	if ok && value == "before" {
		item.SetAttribute("data-state", "after")
	}
}`}}
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher()}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, ok := item.Attribute("data-state"); !ok || got != "after" {
		t.Fatalf("data-state = (%q, %v), want (after, true)", got, ok)
	}
}

func TestRuntimeAddsAndRemovesClasses(t *testing.T) {
	document := dommodel.NewDocument()
	item := document.CreateElement("li", map[string]string{"id": "item", "class": "todo pending"})
	if err := document.AppendChild(document.Root, item); err != nil {
		t.Fatal(err)
	}
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	item := dom.GetElementByID("item")
	item.AddClass("completed")
	item.RemoveClass("pending")
}`}}
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher()}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, ok := item.Attribute("class"); !ok || got != "todo completed" {
		t.Fatalf("class = (%q, %v), want (todo completed, true)", got, ok)
	}
}

func TestRuntimeGetsAndSetsInputValue(t *testing.T) {
	document := dommodel.NewDocument()
	input := document.CreateElement("input", map[string]string{"id": "input", "value": "before"})
	if err := document.AppendChild(document.Root, input); err != nil {
		t.Fatal(err)
	}
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	input := dom.GetElementByID("input")
	if input.Value() == "before" {
		input.SetValue("after")
	}
}`}}
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher()}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := forms.CurrentValue(input); got != "after" {
		t.Fatalf("input value = %q, want after", got)
	}
}

func TestRuntimeReceivesClickEventData(t *testing.T) {
	document := dommodel.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "save"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{button, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	button := dom.GetElementByID("save")
	message := dom.GetElementByID("message")
	button.OnClickEvent(func(event dom.Event) {
		if event.Type == "click" && event.TargetID == "save" && event.X == 12 && event.Y == 34 {
			message.SetText("received")
		}
	})
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !dispatcher.Dispatch(events.Event{Type: events.Click, Target: button.ID, X: 12, Y: 34}) {
		t.Fatal("click event was not handled")
	}
	if got, want := message.TextContent(), "received"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestRuntimeReceivesFocusBlurAndResetEvents(t *testing.T) {
	document := dommodel.NewDocument()
	form := document.CreateElement("form", map[string]string{"id": "form"})
	input := document.CreateElement("input", map[string]string{"id": "name"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	if err := document.AppendChild(document.Root, form); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(form, input); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(document.Root, message); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	input := dom.GetElementByID("name")
	form := dom.GetElementByID("form")
	message := dom.GetElementByID("message")
	input.OnFocus(func(event dom.Event) { if event.Type == "focus" { message.SetText("focus") } })
	input.OnBlur(func(event dom.Event) { if event.Type == "blur" { message.SetText("blur") } })
	form.OnReset(func(event dom.Event) { if event.Type == "reset" { message.SetText("reset") } })
}`}}
	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{Document: document, Events: dispatcher}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, event := range []events.Event{{Type: events.Focus, Target: input.ID}, {Type: events.Blur, Target: input.ID}, {Type: events.Reset, Target: form.ID}} {
		if !dispatcher.Dispatch(event) {
			t.Fatalf("%s was not handled", event.Type)
		}
	}
	if got := message.TextContent(); got != "reset" {
		t.Fatalf("message = %q, want reset", got)
	}
}

func TestRuntimeSubmitHandlerCanPreventDefault(t *testing.T) {
	document := dommodel.NewDocument()
	form := document.CreateElement("form", map[string]string{"id": "form"})
	if err := document.AppendChild(document.Root, form); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	dom.GetElementByID("form").OnSubmit(func(event dom.Event) { event.PreventDefault() })
}`}}
	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{Document: document, Events: dispatcher}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	submit := events.Cancelable(events.Submit, form.ID)
	dispatcher.Dispatch(submit)
	if !submit.DefaultPrevented() {
		t.Fatal("WebGo submit handler did not prevent default")
	}
}

func TestRuntimeReceivesInputEvent(t *testing.T) {
	document := dommodel.NewDocument()
	input := document.CreateElement("input", map[string]string{"id": "query"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{input, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	dom.GetElementByID("query").OnInput(func(event dom.Event) {
		if event.Type == "input" {
			dom.GetElementByID("message").SetText(event.Value)
		}
	})
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !document.SetAttribute(input.ID, "value", "gopher") {
		t.Fatal("SetAttribute(value) = false, want true")
	}
	if !dispatcher.Dispatch(events.Event{Type: events.Input, Target: input.ID, Value: "gopher"}) {
		t.Fatal("input event was not handled")
	}
	if got, want := message.TextContent(), "gopher"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestRuntimeReceivesChangeEvent(t *testing.T) {
	document := dommodel.NewDocument()
	input := document.CreateElement("input", map[string]string{"id": "query", "value": "done"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{input, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	dom.GetElementByID("query").OnChange(func(event dom.Event) {
		dom.GetElementByID("message").SetText(event.Type + ":" + event.Value)
	})
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !dispatcher.Dispatch(events.Event{Type: events.Change, Target: input.ID, Value: "done"}) {
		t.Fatal("change event was not handled")
	}
	if got, want := message.TextContent(), "change:done"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestRuntimeReceivesSubmitEvent(t *testing.T) {
	document := dommodel.NewDocument()
	form := document.CreateElement("form", map[string]string{"id": "todo-form"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{form, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	dom.GetElementByID("todo-form").OnSubmit(func(event dom.Event) {
		dom.GetElementByID("message").SetText(event.Type + ":" + event.TargetID)
	})
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !dispatcher.Dispatch(events.Event{Type: events.Submit, Target: form.ID}) {
		t.Fatal("submit event was not handled")
	}
	if got, want := message.TextContent(), "submit:todo-form"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestRuntimeRegistersEventOnDynamicallyAddedElement(t *testing.T) {
	document := dommodel.NewDocument()
	container := document.CreateElement("main", map[string]string{"id": "container"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{container, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	button := dom.CreateElement("button")
	button.SetAttribute("id", "dynamic-button")
	button.SetText("Run")
	dom.GetElementByID("container").AppendChild(button)
	button.OnClick(func() {
		dom.GetElementByID("message").SetText("clicked")
	})
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	button, ok := document.GetElementByID("dynamic-button")
	if !ok {
		t.Fatal("dynamic button was not appended")
	}
	if !dispatcher.Dispatch(events.Event{Type: events.Click, Target: button.ID}) {
		t.Fatal("dynamic button click was not handled")
	}
	if got, want := message.TextContent(), "clicked"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestRuntimeDoesNotInvokeEventAfterElementRemoval(t *testing.T) {
	document := dommodel.NewDocument()
	container := document.CreateElement("main", map[string]string{"id": "container"})
	removeButton := document.CreateElement("button", map[string]string{"id": "remove"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{container, removeButton, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	item := dom.CreateElement("button")
	item.SetAttribute("id", "temporary")
	dom.GetElementByID("container").AppendChild(item)
	item.OnClick(func() {
		dom.GetElementByID("message").SetText("unexpected")
	})
	dom.GetElementByID("remove").OnClick(func() {
		item.Remove()
	})
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	item, ok := document.GetElementByID("temporary")
	if !ok {
		t.Fatal("temporary element was not appended")
	}
	if !dispatcher.Dispatch(events.Event{Type: events.Click, Target: removeButton.ID}) {
		t.Fatal("remove click was not handled")
	}
	if _, ok := document.GetElementByID("temporary"); ok {
		t.Fatal("temporary element remains after Remove")
	}
	if dispatcher.Dispatch(events.Event{Type: events.Click, Target: item.ID}) {
		t.Fatal("removed element event was handled")
	}
	if got := message.TextContent(); got != "" {
		t.Fatalf("message = %q, want empty", got)
	}
}

func TestRuntimeContainsWebGoHandlerPanicAndContinues(t *testing.T) {
	document := dommodel.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "run"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{button, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	button := dom.GetElementByID("run")
	button.OnClick(func() { panic("webgo boom") })
	button.OnClick(func() {
		dom.GetElementByID("message").SetText("continued")
	})
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !dispatcher.Dispatch(events.Event{Type: events.Click, Target: button.ID}) {
		t.Fatal("click event was not handled")
	}
	if got, want := message.TextContent(), "continued"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestRuntimeDispatchesHoverEventsAndContainsPanic(t *testing.T) {
	document := dommodel.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "run"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{button, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	button := dom.GetElementByID("run")
	message := dom.GetElementByID("message")
	button.OnMouseEnter(func(event dom.Event) { panic("hover boom") })
	button.OnMouseEnter(func(event dom.Event) {
		if event.Type == "mouseenter" && event.TargetID == "run" && event.X == 12 && event.Y == 34 {
			message.SetText("entered")
		}
	})
	button.OnMouseLeave(func(event dom.Event) {
		if event.Type == "mouseleave" {
			message.SetText("left")
		}
	})
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !dispatcher.Dispatch(events.Event{Type: events.MouseEnter, Target: button.ID, X: 12, Y: 34}) {
		t.Fatal("mouseenter event was not handled")
	}
	if got, want := message.TextContent(), "entered"; got != want {
		t.Fatalf("message after enter = %q, want %q", got, want)
	}
	if !dispatcher.Dispatch(events.Event{Type: events.MouseLeave, Target: button.ID, X: 56, Y: 78}) {
		t.Fatal("mouseleave event was not handled")
	}
	if got, want := message.TextContent(), "left"; got != want {
		t.Fatalf("message after leave = %q, want %q", got, want)
	}
}

func TestRuntimeDispatchesWebGoOnClick(t *testing.T) {
	document := dommodel.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "increment"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{button, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	if err := document.AppendChild(message, document.CreateText("before")); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	button := dom.GetElementByID("increment")
	message := dom.GetElementByID("message")
	button.OnClick(func() { message.SetText("clicked") })
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !dispatcher.Dispatch(events.Event{Type: events.Click, Target: button.ID}) {
		t.Fatal("click event was not handled")
	}
	if got, want := message.TextContent(), "clicked"; got != want {
		t.Fatalf("TextContent() = %q, want %q", got, want)
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
