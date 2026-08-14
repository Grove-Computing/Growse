package browser

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

type runtimeStub struct {
	loadCalls     int
	startCalls    int
	stopCalls     int
	loadErr       error
	startErr      error
	environment   runtimemodel.Environment
	mutateOnStart bool
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
	return runtime.startErr
}

func (runtime *runtimeStub) Stop() error {
	runtime.stopCalls++
	return nil
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
