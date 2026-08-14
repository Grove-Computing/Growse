package browser

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	runtimeyaegi "github.com/Grove-Computing/Growse/internal/runtime/yaegi"
)

type runtimeStub struct {
	loadCalls       int
	startCalls      int
	stopCalls       int
	loadErr         error
	startErr        error
	environment     runtimemodel.Environment
	mutateOnStart   bool
	navigateOnStart string
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

func (runtime *runtimeStub) Load(_ context.Context, _ []runtimemodel.Script, environment runtimemodel.Environment) error {
	runtime.loadCalls++
	runtime.environment = environment
	return runtime.loadErr
}

func (runtime *runtimeStub) Start(context.Context) error {
	runtime.startCalls++
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
	runtime.stopCalls++
	return nil
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
	for browser.Page().URL.String() != secondURL.String() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := browser.Page().URL.String(); got != secondURL.String() {
		t.Fatalf("active URL = %q, want %q", got, secondURL)
	}
	if got, want := len(browser.history.entries), 2; got != want {
		t.Fatalf("history entries = %d, want %d", got, want)
	}
	if runtime.stopCalls != 1 {
		t.Fatalf("previous Runtime Stop() calls = %d, want 1", runtime.stopCalls)
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
	if runtime.loadCalls != 1 || runtime.startCalls != 1 {
		t.Fatalf("runtime calls = load:%d start:%d, want 1 each", runtime.loadCalls, runtime.startCalls)
	}
	running, ok := page.Document.GetElementByID("running")
	if !ok || page.Animations.Count(running.ID) != 1 {
		t.Fatal("running CSS animation was not registered")
	}
	if err := browser.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if runtime.stopCalls != 1 {
		t.Fatalf("Stop() calls = %d, want 1", runtime.stopCalls)
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
	if runtime.stopCalls != 1 {
		t.Fatalf("Stop() calls = %d, want failed runtime cleanup", runtime.stopCalls)
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
	if len(runtimes) != 2 || runtimes[0].stopCalls != 1 {
		t.Fatalf("after navigation runtimes = %d first stops = %d", len(runtimes), runtimes[0].stopCalls)
	}
	if _, err := browser.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if len(runtimes) != 3 || runtimes[1].stopCalls != 1 {
		t.Fatalf("after reload runtimes = %d second stops = %d", len(runtimes), runtimes[1].stopCalls)
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
