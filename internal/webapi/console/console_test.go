package console

import "testing"

func TestLogFormatsWebGoMessage(t *testing.T) {
	var message string
	console := New(func(value string) {
		message = value
	})

	console.Log("count=", 3)

	if got, want := message, "[WebGo] count=3"; got != want {
		t.Fatalf("Log() message = %q, want %q", got, want)
	}
}
