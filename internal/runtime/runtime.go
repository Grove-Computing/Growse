// Package runtime はページで使用する交換可能なGo実行環境を定義する。
package runtime

import (
	"context"
	"net/url"
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/network"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
	fetchapi "github.com/Grove-Computing/Growse/internal/webapi/fetch"
)

// Engine はPage Scriptを評価する実行エンジンを識別する。
type Engine string

// ScriptSchedule identifies when a classic script participates in document
// loading. The zero value is parser-blocking for compatibility.
type ScriptSchedule string

// ScriptKind distinguishes classic scripts from ECMAScript modules.
type ScriptKind string

const (
	EngineGo         Engine = "go"
	EngineJavaScript Engine = "javascript"

	ScriptParserBlocking ScriptSchedule = "parser-blocking"
	ScriptDefer          ScriptSchedule = "defer"
	ScriptAsync          ScriptSchedule = "async"

	ScriptClassic ScriptKind = "classic"
	ScriptModule  ScriptKind = "module"
)

// NormalizeEngine は既存呼び出しのzero valueをGoとして扱う。
func NormalizeEngine(engine Engine) Engine {
	if engine == "" {
		return EngineGo
	}
	return engine
}

// Valid はGrowseが選択可能なEngineかを返す。
func (engine Engine) Valid() bool {
	switch NormalizeEngine(engine) {
	case EngineGo, EngineJavaScript:
		return true
	default:
		return false
	}
}

// Script は文書内で見つかった1つの実行ソースを表す。
type Script struct {
	Engine        Engine
	Kind          ScriptKind
	SourceURL     *url.URL
	Source        string
	Inline        bool
	Integrity     string
	CrossOrigin   string
	Schedule      ScriptSchedule
	DocumentOrder int
	FetchOrder    int
}

// SandboxStatus reports the process boundary that was verified before page
// code was accepted by a Runtime. Constraints only contains OS controls that
// the worker actually applied; optional controls are never treated as required.
type SandboxStatus struct {
	Platform           string   `json:"platform"`
	Ready              bool     `json:"ready"`
	ProcessBoundary    bool     `json:"processBoundary"`
	BrokeredHostIO     bool     `json:"brokeredHostIo"`
	MinimalEnvironment bool     `json:"minimalEnvironment"`
	ParentLifecycle    bool     `json:"parentLifecycle"`
	MemoryLimitBytes   int64    `json:"memoryLimitBytes"`
	Constraints        []string `json:"constraints,omitempty"`
	Failure            string   `json:"failure,omitempty"`
}

// Environment はRuntimeへ公開するページの状態を保持する。
type Environment struct {
	Document        *dom.Document
	Events          *events.Dispatcher
	BaseURL         *url.URL
	ImportMap       map[string]string
	Fetch           func(context.Context, *network.Request) (*network.Response, error)
	FetchLimiter    *fetchapi.Limiter
	Navigate        func(*url.URL) error
	HistoryPush     func(string, *url.URL) error
	HistoryReplace  func(string, *url.URL) error
	HistoryTraverse func(int) error
	HistoryInfo     func() (int, string)
	LocalStorage    *storagecore.Area
	SessionStorage  *storagecore.Area
	StorageSource   storagecore.MutationSource
	OnMutation      func()
	RequestFrame    func()
	FrameScope      func(time.Time, func())
	ConsoleLog      func(message string)
	ConsoleRecord   func(level, message string)
	RuntimeFailure  func(error)
	Frames          []FrameAccess
	FrameMutation   func(frameID, generation uint64, document dom.DocumentSnapshot) error
}

// FrameAccess is the least-privilege view of one direct child browsing context.
// Document is present only while same-origin access is allowed.
type FrameAccess struct {
	ID         uint64
	ElementID  dom.NodeID
	Generation uint64
	Origin     string
	URL        string
	SameOrigin bool
	Document   *dom.Document
}

// Runtime は1ページに属するGoスクリプトを実行する。
type Runtime interface {
	Load(ctx context.Context, scripts []Script, environment Environment) error
	Start(ctx context.Context) error
	Stop() error
}

// SandboxReporter is implemented by runtimes that verify an out-of-process
// sandbox before evaluating page code.
type SandboxReporter interface {
	SandboxStatus() SandboxStatus
}

// LocationUpdater はsame-document Navigationを現在Runtimeへ通知する。
type LocationUpdater interface {
	UpdateLocation(*url.URL)
}

// FrameUpdater receives generation-scoped child Frame access changes.
type FrameUpdater interface {
	UpdateFrames([]FrameAccess)
}

// NavigationEventDispatcher はNavigation Eventを現在Runtimeへ配送する。
type NavigationEventDispatcher interface {
	DispatchPopState(state string)
	DispatchHashChange(oldURL, newURL string)
}

// Factory はページごとに独立したRuntimeを生成する。
type Factory func() Runtime

// EngineFactory は選択Engineに対応する独立したRuntimeを生成する。
type EngineFactory func(Engine) Runtime

// ForGo は既存のGo専用Factoryを言語選択可能なFactoryへ変換する。
// JavaScriptを要求された場合はnilを返し、暗黙にGoへfallbackしない。
func ForGo(factory Factory) EngineFactory {
	return func(engine Engine) Runtime {
		if NormalizeEngine(engine) != EngineGo || factory == nil {
			return nil
		}
		return factory()
	}
}
