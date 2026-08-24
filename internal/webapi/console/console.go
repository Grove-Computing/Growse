// Package console はWebGoスクリプト向けのログAPIを提供する。
package console

import (
	"fmt"
	"log/slog"
)

// Console は1ページのWebGoログを出力する。
type Console struct {
	sink func(level Level, message string)
}

// Level identifies the severity selected by a WebGo console call.
type Level string

const (
	LevelLog   Level = "log"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// New は指定した出力先を使うConsoleを生成する。
func New(sink func(message string)) *Console {
	if sink == nil {
		return NewWithLevelSink(nil)
	}
	return NewWithLevelSink(func(_ Level, message string) { sink(message) })
}

// NewWithLevelSink creates a Console that preserves message severity.
func NewWithLevelSink(sink func(level Level, message string)) *Console {
	if sink == nil {
		sink = func(level Level, message string) {
			slog.Info(message, "component", "runtime", "level", level)
		}
	}
	return &Console{sink: sink}
}

// Log は値を連結し、WebGoログとして出力する。
func (console *Console) Log(values ...any) {
	console.write(LevelLog, values...)
}

// Info emits an informational WebGo message.
func (console *Console) Info(values ...any) {
	console.write(LevelInfo, values...)
}

// Warn emits a warning WebGo message.
func (console *Console) Warn(values ...any) {
	console.write(LevelWarn, values...)
}

// Error emits an error WebGo message.
func (console *Console) Error(values ...any) {
	console.write(LevelError, values...)
}

func (console *Console) write(level Level, values ...any) {
	console.sink(level, "[WebGo] "+fmt.Sprint(values...))
}
