package events

import (
	"reflect"
	"testing"

	"github.com/saku0512/growse/internal/dom"
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
	dispatcher := NewDispatcher()
	called := false
	dispatcher.AddEventListener(1, Click, func(Event) { panic("boom") })
	dispatcher.AddEventListener(1, Click, func(Event) { called = true })

	if !dispatcher.Dispatch(Event{Type: Click, Target: 1}) || !called {
		t.Fatal("Dispatch() did not continue after listener panic")
	}
}

func TestDispatchReportsUnhandledEvent(t *testing.T) {
	if NewDispatcher().Dispatch(Event{Type: Click, Target: 1}) {
		t.Fatal("Dispatch() = true for event without listeners")
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
