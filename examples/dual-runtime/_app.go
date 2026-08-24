package main

import (
	"growse/console"
	"growse/dom"
	"growse/fetch"
	"growse/navigation"
	"growse/scheduler"
	"growse/storage"
	"growse/strconv"
)

var count int

func main() {
	setText("engine", "go")
	console.Info("dual-runtime engine=go")
	if increment := dom.GetElementByID("increment"); increment != nil {
		increment.OnClick(func() {
			count++
			setText("count", strconv.Itoa(count))
		})
	}
	if note := dom.GetElementByID("note"); note != nil {
		if saved, ok, _ := storage.Local().Get("dual-note"); ok {
			note.SetValue(saved)
			setText("storage", saved)
		}
		note.OnInput(func(event dom.Event) {
			_ = storage.Local().Set("dual-note", event.Value)
			setText("storage", event.Value)
		})
	}
	_, _ = scheduler.SetTimeout(10*scheduler.Millisecond, func() { setText("timer", "go timer fired") })
	fetch.Fetch(fetch.Request{URL: "/api/message"}, func(response fetch.Response) {
		message, err := response.Text()
		if err != nil {
			setText("fetch-success", err.Error())
			return
		}
		setText("fetch-success", message)
	}, func(message string) { setText("fetch-success", message) })
	fetch.Fetch(fetch.Request{URL: "/api/failure"}, func(response fetch.Response) {
		setText("fetch-failure", "HTTP "+strconv.Itoa(response.Status))
	}, func(message string) { setText("fetch-failure", message) })
	if route := dom.GetElementByID("route"); route != nil {
		route.OnClick(func() {
			_ = navigation.PushState(`{"engine":"go"}`, "?view=go#history")
			setText("location", navigation.Current().Href)
		})
	}
	if runtimeError := dom.GetElementByID("runtime-error"); runtimeError != nil {
		runtimeError.OnClick(func() { console.Error("intentional Go showcase error") })
	}
}

func setText(id, value string) {
	if element := dom.GetElementByID(id); element != nil {
		element.SetText(value)
	}
}
