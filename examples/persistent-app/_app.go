package main

import (
	"growse/dom"
	"growse/fetch"
	"growse/navigation"
	"growse/scheduler"
	"growse/storage"
)

var app *dom.Element
var input *dom.Element
var note *dom.Element
var route *dom.Element
var status *dom.Element
var saveTimer scheduler.TimerID
var currentNote string

func main() {
	app = dom.GetElementByID("app")
	input = dom.GetElementByID("note-input")
	note = dom.GetElementByID("note")
	route = dom.GetElementByID("route")
	status = dom.GetElementByID("status")

	currentNote, _, _ = storage.Local().Get("current-note")
	if draft, ok, _ := storage.Session().Get("draft"); ok {
		currentNote = draft
	}
	renderNote()
	if currentNote != "" {
		setStatus("cache-revalidating")
	}
	if input != nil {
		input.SetValue(currentNote)
		input.OnInput(queueSave)
	}
	if form := dom.GetElementByID("note-form"); form != nil {
		form.OnSubmit(submitNote)
	}
	if all := dom.GetElementByID("all-notes"); all != nil {
		all.OnClick(showAll)
	}
	navigation.OnPopState(func(navigation.PopStateEvent) { renderRoute() })
	navigation.OnHashChange(func(navigation.HashChangeEvent) { renderRoute() })
	_ = navigation.ReplaceState(`{"view":"notes"}`, "")
	renderRoute()
	fetch.Fetch(fetch.Request{URL: "/api/notes"}, synced, syncFailed)
}

func queueSave(event dom.Event) {
	currentNote = event.Value
	_ = storage.Session().Set("draft", currentNote)
	renderNote()
	setStatus("saving")
	if saveTimer != 0 {
		scheduler.ClearTimer(saveTimer)
	}
	saveTimer, _ = scheduler.SetTimeout(25*scheduler.Millisecond, save)
}

func submitNote(event dom.Event) {
	event.PreventDefault()
	if input != nil {
		currentNote = input.Value()
	}
	save()
}

func save() {
	saveTimer = 0
	_ = storage.Local().Set("current-note", currentNote)
	_ = storage.Session().Remove("draft")
	setStatus("saved")
	_, _ = scheduler.RequestAnimationFrame(func(timestamp scheduler.Timestamp) {
		_ = timestamp
		if app != nil {
			app.AddClass("frame-committed")
		}
	})
}

func showAll() {
	_ = navigation.PushState(`{"filter":"all"}`, "?filter=all#notes")
	renderRoute()
}

func renderRoute() {
	if route == nil {
		return
	}
	location := navigation.Current()
	value := "route: all"
	if location.Query != "" {
		value = "route: " + location.Query
	}
	if location.Fragment != "" {
		value = value + " #" + location.Fragment
	}
	route.SetText(value)
}

func renderNote() {
	if note == nil {
		return
	}
	if currentNote == "" {
		note.SetAttribute("class", "note empty")
		note.SetText("No notes yet")
		return
	}
	note.SetAttribute("class", "note")
	note.SetText(currentNote)
}

func synced(response fetch.Response) {
	if response.Status == 409 {
		setStatus("conflict")
		return
	}
	if response.Status >= 500 {
		syncFailed("server unavailable")
		return
	}
	remote, err := response.Text()
	if err != nil {
		syncFailed(err.Error())
		return
	}
	if currentNote == "" && remote != "" {
		currentNote = remote
		_ = storage.Local().Set("current-note", currentNote)
		if input != nil {
			input.SetValue(currentNote)
		}
		renderNote()
	}
	if currentNote == "" {
		setStatus("empty")
		return
	}
	setStatus("synced")
}

func syncFailed(string) {
	if currentNote != "" {
		setStatus("offline")
		return
	}
	setStatus("network-error")
}

func setStatus(mode string) {
	if status == nil {
		return
	}
	status.SetAttribute("class", "status "+mode)
	status.SetText(mode)
}
