package main

import (
	"growse/dom"
	"growse/storage"
)

var noteInput *dom.Element
var noteStatus *dom.Element
var sharedNote *dom.Element

func main() {
	noteInput = dom.GetElementByID("note-input")
	noteStatus = dom.GetElementByID("status")
	sharedNote = dom.GetElementByID("shared-note")
	if value, ok, _ := storage.Local().Get("workspace-note"); ok {
		noteInput.SetValue(value)
		showNote(value)
	}
	if draft, ok, _ := storage.Session().Get("note-draft"); ok {
		noteInput.SetValue(draft)
	}
	noteInput.OnInput(func(event dom.Event) {
		_ = storage.Session().Set("note-draft", event.Value)
		noteStatus.SetText("dirty")
		noteStatus.SetAttribute("class", "pill dirty")
	})
	dom.GetElementByID("note-form").OnSubmit(func(event dom.Event) {
		event.PreventDefault()
		value := noteInput.Value()
		_ = storage.Local().Set("workspace-note", value)
		_ = storage.Session().Remove("note-draft")
		showNote(value)
		noteStatus.SetText("shared")
		noteStatus.SetAttribute("class", "pill ready")
	})
	storage.OnChange(func(event storage.Event) {
		if event.Key == "workspace-note" && event.HasNewValue {
			showNote(event.NewValue)
		}
	})
}

func showNote(value string) {
	if value == "" {
		sharedNote.SetText("No shared note yet")
		return
	}
	sharedNote.SetText(value)
}
