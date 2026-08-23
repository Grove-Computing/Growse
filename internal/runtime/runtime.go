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

// Script は文書内で見つかった1つのGoソースを表す。
type Script struct {
	SourceURL *url.URL
	Source    string
	Inline    bool
}

// Environment はRuntimeへ公開するページの状態を保持する。
type Environment struct {
	Document        *dom.Document
	Events          *events.Dispatcher
	BaseURL         *url.URL
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
}

// Runtime は1ページに属するGoスクリプトを実行する。
type Runtime interface {
	Load(ctx context.Context, scripts []Script, environment Environment) error
	Start(ctx context.Context) error
	Stop() error
}

// LocationUpdater はsame-document Navigationを現在Runtimeへ通知する。
type LocationUpdater interface {
	UpdateLocation(*url.URL)
}

// NavigationEventDispatcher はNavigation Eventを現在Runtimeへ配送する。
type NavigationEventDispatcher interface {
	DispatchPopState(state string)
	DispatchHashChange(oldURL, newURL string)
}

// Factory はページごとに独立したRuntimeを生成する。
type Factory func() Runtime
