package browser

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/saku0512/growse/internal/network"
	runtimemodel "github.com/saku0512/growse/internal/runtime"
)

type runtimeStub struct {
	loadCalls  int
	startCalls int
	stopCalls  int
	loadErr    error
	startErr   error
}

func (runtime *runtimeStub) Load(context.Context, []runtimemodel.Script, runtimemodel.Environment) error {
	runtime.loadCalls++
	return runtime.loadErr
}

func (runtime *runtimeStub) Start(context.Context) error {
	runtime.startCalls++
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
		Body: []byte(`<script type="text/go">package main
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
	if err := browser.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if runtime.stopCalls != 1 {
		t.Fatalf("Stop() calls = %d, want 1", runtime.stopCalls)
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
