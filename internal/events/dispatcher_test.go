package events

import (
	"bytes"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestDispatchCallsTargetListenersInRegistrationOrder(t *testing.T) {
	dispatcher := NewDispatcher()
	var calls []int
	dispatcher.AddEventListener(7, Click, func(event Event) {
		if event.X != 12 || event.Y != 34 {
			t.Fatalf("event coordinates = (%v, %v), want (12, 34)", event.X, event.Y)
		}
		calls = append(calls, 1)
	})
	dispatcher.AddEventListener(7, Click, func(Event) { calls = append(calls, 2) })
	dispatcher.AddEventListener(8, Click, func(Event) { calls = append(calls, 3) })

	handled := dispatcher.Dispatch(Event{Type: Click, Target: dom.NodeID(7), X: 12, Y: 34})

	if !handled || !reflect.DeepEqual(calls, []int{1, 2}) {
		t.Fatalf("Dispatch() = %v, calls %v; want true, [1 2]", handled, calls)
	}
}

func TestDispatchContainsListenerPanicAndContinues(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	dispatcher := NewDispatcher()
	called := false
	dispatcher.AddEventListener(1, Click, func(Event) { panic("boom") })
	dispatcher.AddEventListener(1, Click, func(Event) { called = true })

	if !dispatcher.Dispatch(Event{Type: Click, Target: 1}) || !called {
		t.Fatal("Dispatch() did not continue after listener panic")
	}
	for _, want := range []string{"WebGoイベントハンドラーでpanicが発生しました", "type=click", "panic=boom"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("panic log = %q, want to contain %q", logs.String(), want)
		}
	}
}

func TestDispatchReportsUnhandledEvent(t *testing.T) {
	if NewDispatcher().Dispatch(Event{Type: Click, Target: 1}) {
		t.Fatal("Dispatch() = true for event without listeners")
	}
}

func TestCancelableEventSharesPreventDefaultAcrossCopies(t *testing.T) {
	dispatcher := NewDispatcher()
	dispatcher.AddEventListener(1, Submit, func(event Event) { event.PreventDefault() })
	event := Cancelable(Submit, 1)
	if !dispatcher.Dispatch(event) || !event.DefaultPrevented() {
		t.Fatal("cancelable submit event was not prevented")
	}
}

func TestRemoveEventListenersRemovesSpecifiedNodes(t *testing.T) {
	dispatcher := NewDispatcher()
	dispatcher.AddEventListener(1, Click, func(Event) {})
	dispatcher.AddEventListener(2, Click, func(Event) {})

	dispatcher.RemoveEventListeners(1)

	if dispatcher.Dispatch(Event{Type: Click, Target: 1}) {
		t.Fatal("removed node event was handled")
	}
	if !dispatcher.Dispatch(Event{Type: Click, Target: 2}) {
		t.Fatal("unrelated node event listener was removed")
	}
}

func TestDispatchTreeUsesCaptureTargetBubbleAndRemoval(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("main", nil)
	target := document.CreateElement("button", nil)
	if err := document.AppendChild(document.Root, parent); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(parent, target); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewDispatcher()
	var calls []string
	record := func(label string) Listener {
		return func(event Event) {
			calls = append(calls, fmt.Sprintf("%s:%d:%d:%d", label, event.Target, event.CurrentTarget(), event.EventPhase()))
		}
	}
	dispatcher.AddEventListenerWithCapture(parent.ID, Click, true, record("parent-capture"))
	removed := dispatcher.AddEventListenerWithCapture(parent.ID, Click, false, record("removed"))
	dispatcher.AddEventListenerWithCapture(target.ID, Click, true, record("target-capture"))
	dispatcher.AddEventListener(target.ID, Click, record("target-bubble"))
	dispatcher.AddEventListener(parent.ID, Click, record("parent-bubble"))
	if !dispatcher.RemoveEventListener(parent.ID, Click, removed) {
		t.Fatal("RemoveEventListener() = false")
	}
	if !dispatcher.DispatchTree(document, Cancelable(Click, target.ID)) {
		t.Fatal("DispatchTree() = false")
	}
	want := []string{
		fmt.Sprintf("parent-capture:%d:%d:1", target.ID, parent.ID),
		fmt.Sprintf("target-capture:%d:%d:2", target.ID, target.ID),
		fmt.Sprintf("target-bubble:%d:%d:2", target.ID, target.ID),
		fmt.Sprintf("parent-bubble:%d:%d:3", target.ID, parent.ID),
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("propagation calls = %v, want %v", calls, want)
	}
}

func TestDispatchTreeStopPropagationAndNonBubblingEvent(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("main", nil)
	target := document.CreateElement("input", nil)
	_ = document.AppendChild(document.Root, parent)
	_ = document.AppendChild(parent, target)
	dispatcher := NewDispatcher()
	var calls []string
	dispatcher.AddEventListener(target.ID, Click, func(event Event) { calls = append(calls, "first"); event.StopPropagation() })
	dispatcher.AddEventListener(target.ID, Click, func(Event) { calls = append(calls, "second") })
	dispatcher.AddEventListener(parent.ID, Click, func(Event) { calls = append(calls, "parent") })
	dispatcher.DispatchTree(document, Event{Type: Click, Target: target.ID})
	if !reflect.DeepEqual(calls, []string{"first", "second"}) {
		t.Fatalf("stopped propagation calls = %v", calls)
	}
	calls = nil
	dispatcher.AddEventListenerWithCapture(parent.ID, Focus, true, func(Event) { calls = append(calls, "capture") })
	dispatcher.AddEventListener(target.ID, Focus, func(Event) { calls = append(calls, "target") })
	dispatcher.AddEventListener(parent.ID, Focus, func(Event) { calls = append(calls, "bubble") })
	dispatcher.DispatchTree(document, Event{Type: Focus, Target: target.ID})
	if !reflect.DeepEqual(calls, []string{"capture", "target"}) {
		t.Fatalf("non-bubbling focus calls = %v", calls)
	}
}
