// Package yaegi はYaegiを使ったGrowseページのRuntimeを実装する。
package yaegi

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"strings"
	"sync"
	"testing/fstest"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	consoleapi "github.com/Grove-Computing/Growse/internal/webapi/console"
	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	fetchapi "github.com/Grove-Computing/Growse/internal/webapi/fetch"
	navigationapi "github.com/Grove-Computing/Growse/internal/webapi/navigation"
	schedulerapi "github.com/Grove-Computing/Growse/internal/webapi/scheduler"
	strconvapi "github.com/Grove-Computing/Growse/internal/webapi/strconv"
	"github.com/traefik/yaegi/interp"
)

// Runtime は1ページ専用のYaegiインタープリターである。
type Runtime struct {
	mu            sync.Mutex
	executionMu   sync.Mutex
	interpreter   *interp.Interpreter
	cancel        context.CancelFunc
	runtimeCtx    context.Context
	callbackQueue chan func()
	callbackDone  chan struct{}
	fetchAPI      *fetchapi.API
	schedulerAPI  *schedulerapi.API
	loaded        bool
	started       bool
	stopped       bool
}

// New は未ロードのYaegi Runtimeを生成する。
func New() *Runtime {
	return &Runtime{}
}

// Load はスクリプトを検証し、実行せずにインタープリターを準備する。
func (r *Runtime) Load(ctx context.Context, scripts []runtimemodel.Script, environment runtimemodel.Environment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(scripts) == 0 {
		return errors.New("no Go scripts to load")
	}

	files := make(fstest.MapFS, len(scripts))
	mainCount := 0
	for index, script := range scripts {
		name := fmt.Sprintf("script_%03d.go", index)
		parsed, err := parser.ParseFile(token.NewFileSet(), name, script.Source, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", scriptName(script, name), err)
		}
		if parsed.Name.Name != "main" {
			return fmt.Errorf("%s uses package %q; want package main", scriptName(script, name), parsed.Name.Name)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && function.Name.Name == "main" {
				mainCount++
			}
		}
		files["src/main/page/"+name] = &fstest.MapFile{Data: []byte(script.Source)}
	}
	if mainCount != 1 {
		return fmt.Errorf("go scripts define %d main functions; want exactly 1", mainCount)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return errors.New("runtime already started")
	}
	if r.stopped {
		return errors.New("runtime already stopped")
	}
	r.interpreter = interp.New(interp.Options{GoPath: ".", SourcecodeFilesystem: portableFS{FS: files}})
	r.runtimeCtx, r.cancel = context.WithCancel(ctx)
	r.callbackQueue = make(chan func(), 32)
	r.callbackDone = make(chan struct{})
	go r.runCallbacks(r.runtimeCtx, r.callbackQueue, r.callbackDone)
	console := consoleapi.New(environment.ConsoleLog)
	dom := domapi.New(environment.Document, environment.Events, environment.OnMutation)
	fetch := fetchapi.NewPage(r.runtimeCtx, environment.BaseURL, environment.Fetch, r.enqueueCallback)
	navigation := navigationapi.NewPage(environment.BaseURL, environment.Navigate)
	r.fetchAPI = fetch
	scheduler := schedulerapi.NewPage(r.runtimeCtx, r.enqueueCallback, environment.RequestFrame)
	scheduler.SetFrameScope(environment.FrameScope)
	r.schedulerAPI = scheduler
	if err := r.interpreter.Use(interp.Exports{
		"growse/console/console": {
			"Log": reflect.ValueOf(console.Log),
		},
		"growse/dom/dom": {
			"CreateElement":  reflect.ValueOf(dom.CreateElement),
			"Element":        reflect.ValueOf((*domapi.Element)(nil)),
			"Event":          reflect.ValueOf((*domapi.Event)(nil)),
			"GetElementByID": reflect.ValueOf(dom.GetElementByID),
			"QuerySelector":  reflect.ValueOf(dom.QuerySelector),
		},
		"growse/fetch/fetch": {
			"CredentialsInclude":    reflect.ValueOf(fetchapi.CredentialsInclude),
			"CredentialsMode":       reflect.ValueOf((*fetchapi.CredentialsMode)(nil)),
			"CredentialsOmit":       reflect.ValueOf(fetchapi.CredentialsOmit),
			"CredentialsSameOrigin": reflect.ValueOf(fetchapi.CredentialsSameOrigin),
			"Fetch":                 reflect.ValueOf(fetch.Fetch),
			"Header":                reflect.ValueOf((*fetchapi.Header)(nil)),
			"Request":               reflect.ValueOf((*fetchapi.Request)(nil)),
			"Response":              reflect.ValueOf((*fetchapi.Response)(nil)),
		},
		"growse/navigation/navigation": {
			"Current":  reflect.ValueOf(navigation.Current),
			"Location": reflect.ValueOf((*navigationapi.Location)(nil)),
			"Navigate": reflect.ValueOf(navigation.Navigate),
			"Resolve":  reflect.ValueOf(navigation.Resolve),
		},
		"growse/scheduler/scheduler": {
			"CancelAnimationFrame":  reflect.ValueOf(scheduler.CancelAnimationFrame),
			"ClearTimer":            reflect.ValueOf(scheduler.ClearTimer),
			"FrameID":               reflect.ValueOf((*schedulerapi.FrameID)(nil)),
			"Millisecond":           reflect.ValueOf(schedulerapi.Millisecond),
			"RequestAnimationFrame": reflect.ValueOf(scheduler.RequestAnimationFrame),
			"Second":                reflect.ValueOf(schedulerapi.Second),
			"SetInterval":           reflect.ValueOf(scheduler.SetInterval),
			"SetTimeout":            reflect.ValueOf(scheduler.SetTimeout),
			"TimerID":               reflect.ValueOf((*schedulerapi.TimerID)(nil)),
			"Timestamp":             reflect.ValueOf((*schedulerapi.Timestamp)(nil)),
		},
		"growse/strconv/strconv": {
			"Itoa": reflect.ValueOf(strconvapi.Itoa),
		},
	}); err != nil {
		return fmt.Errorf("register Growse Web API: %w", err)
	}
	r.loaded = true
	return nil
}

// RunAnimationFrame synchronously delivers one frame through the page queue.
func (r *Runtime) RunAnimationFrame(current time.Time) bool {
	r.mu.Lock()
	scheduler := r.schedulerAPI
	ctx := r.runtimeCtx
	r.mu.Unlock()
	if scheduler == nil || ctx == nil || !scheduler.HasAnimationFrameCallbacks() {
		return false
	}
	result := make(chan bool, 1)
	if !r.enqueueCallback(func() {
		result <- scheduler.RunAnimationFrame(current)
	}) {
		return false
	}
	select {
	case ran := <-result:
		return ran
	case <-ctx.Done():
		return false
	}
}

// HasAnimationFrameCallbacks reports whether the page requested another frame.
func (r *Runtime) HasAnimationFrameCallbacks() bool {
	r.mu.Lock()
	scheduler := r.schedulerAPI
	r.mu.Unlock()
	return scheduler != nil && scheduler.HasAnimationFrameCallbacks()
}

// DispatchPageEvent runs a browser-originated DOM event on the page queue and
// waits for cancelation/default-action state to become observable.
func (r *Runtime) DispatchPageEvent(callback func() bool) bool {
	if callback == nil {
		return false
	}
	r.mu.Lock()
	ctx := r.runtimeCtx
	r.mu.Unlock()
	if ctx == nil {
		return false
	}
	result := make(chan bool, 1)
	if !r.enqueueCallback(func() {
		result <- callback()
	}) {
		return false
	}
	select {
	case handled := <-result:
		return handled
	case <-ctx.Done():
		return false
	}
}

// portableFS はOS固有の区切り文字をio/fs形式へ正規化する。
type portableFS struct {
	fs.FS
}

func (filesystem portableFS) Open(name string) (fs.File, error) {
	return filesystem.FS.Open(strings.ReplaceAll(name, `\`, "/"))
}

// Start はロード済みの全ファイルを評価し、main.mainを一度呼び出す。
func (r *Runtime) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	if !r.loaded || r.interpreter == nil {
		r.mu.Unlock()
		return errors.New("runtime is not loaded")
	}
	if r.started {
		r.mu.Unlock()
		return errors.New("runtime already started")
	}
	if r.stopped {
		r.mu.Unlock()
		return errors.New("runtime already stopped")
	}
	r.started = true
	interpreter := r.interpreter
	runtimeContext := r.runtimeCtx
	r.mu.Unlock()

	r.executionMu.Lock()
	_, err := interpreter.EvalPathWithContext(runtimeContext, "page")
	r.executionMu.Unlock()
	if err != nil {
		return fmt.Errorf("execute Go scripts: %w", err)
	}
	return nil
}

// Stop は実行中の処理を中止する。複数回呼び出しても安全である。
func (r *Runtime) Stop() error {
	r.mu.Lock()
	cancel := r.cancel
	r.cancel = nil
	r.stopped = true
	done := r.callbackDone
	fetch := r.fetchAPI
	scheduler := r.schedulerAPI
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if fetch != nil {
		fetch.Close()
	}
	if scheduler != nil {
		scheduler.Close()
	}
	if done != nil {
		<-done
	}
	return nil
}

func (r *Runtime) enqueueCallback(callback func()) bool {
	r.mu.Lock()
	ctx := r.runtimeCtx
	queue := r.callbackQueue
	stopped := r.stopped
	r.mu.Unlock()
	if callback == nil || ctx == nil || queue == nil || stopped {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	default:
	}
	select {
	case queue <- callback:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *Runtime) runCallbacks(ctx context.Context, queue <-chan func(), done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		select {
		case <-ctx.Done():
			return
		case callback := <-queue:
			if callback == nil {
				continue
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
			r.executionMu.Lock()
			select {
			case <-ctx.Done():
				r.executionMu.Unlock()
				return
			default:
			}
			callback()
			r.executionMu.Unlock()
		}
	}
}

func scriptName(script runtimemodel.Script, fallback string) string {
	if script.SourceURL == nil {
		return fallback
	}
	return network.RedactedURL(script.SourceURL)
}
