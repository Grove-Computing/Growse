// Package yaegi はYaegiを使ったGrowseページのRuntimeを実装する。
package yaegi

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sync"
	"testing/fstest"

	runtimemodel "github.com/saku0512/growse/internal/runtime"
	consoleapi "github.com/saku0512/growse/internal/webapi/console"
	"github.com/traefik/yaegi/interp"
)

// Runtime は1ページ専用のYaegiインタープリターである。
type Runtime struct {
	mu          sync.Mutex
	interpreter *interp.Interpreter
	cancel      context.CancelFunc
	loaded      bool
	started     bool
	stopped     bool
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
	r.interpreter = interp.New(interp.Options{GoPath: ".", SourcecodeFilesystem: files})
	console := consoleapi.New(environment.ConsoleLog)
	if err := r.interpreter.Use(interp.Exports{
		"growse/console/console": {
			"Log": reflect.ValueOf(console.Log),
		},
	}); err != nil {
		return fmt.Errorf("register growse/console: %w", err)
	}
	r.loaded = true
	return nil
}

// Start はロード済みの全ファイルを評価し、main.mainを一度呼び出す。
func (r *Runtime) Start(ctx context.Context) error {
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
	runtimeContext, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.started = true
	interpreter := r.interpreter
	r.mu.Unlock()

	_, err := interpreter.EvalPathWithContext(runtimeContext, "page")

	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	r.mu.Unlock()
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
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func scriptName(script runtimemodel.Script, fallback string) string {
	if script.SourceURL == nil {
		return fallback
	}
	return script.SourceURL.Redacted()
}
