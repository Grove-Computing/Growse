// Package console はWebGoスクリプト向けのログAPIを提供する。
package console

import (
	"fmt"
	"log/slog"
)

// Console は1ページのWebGoログを出力する。
type Console struct {
	sink func(message string)
}

// New は指定した出力先を使うConsoleを生成する。
func New(sink func(message string)) *Console {
	if sink == nil {
		sink = func(message string) {
			slog.Info(message, "component", "runtime")
		}
	}
	return &Console{sink: sink}
}

// Log は値を連結し、WebGoログとして出力する。
func (console *Console) Log(values ...any) {
	console.sink("[WebGo] " + fmt.Sprint(values...))
}
