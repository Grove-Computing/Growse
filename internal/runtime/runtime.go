// Package runtime はページで使用する交換可能なGo実行環境を定義する。
package runtime

import (
	"context"
	"net/url"

	"github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/events"
)

// Script は文書内で見つかった1つのGoソースを表す。
type Script struct {
	SourceURL *url.URL
	Source    string
	Inline    bool
}

// Environment はRuntimeへ公開するページの状態を保持する。
type Environment struct {
	Document   *dom.Document
	Events     *events.Dispatcher
	BaseURL    *url.URL
	OnMutation func()
	ConsoleLog func(message string)
}

// Runtime は1ページに属するGoスクリプトを実行する。
type Runtime interface {
	Load(ctx context.Context, scripts []Script, environment Environment) error
	Start(ctx context.Context) error
	Stop() error
}

// Factory はページごとに独立したRuntimeを生成する。
type Factory func() Runtime
