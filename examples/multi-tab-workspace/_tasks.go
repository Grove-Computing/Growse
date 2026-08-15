package main

import (
	"growse/dom"
	"growse/scheduler"
	"growse/storage"
	"growse/strconv"
)

var taskInput *dom.Element
var taskCount *dom.Element
var tasks int

func main() {
	taskInput = dom.GetElementByID("task-input")
	taskCount = dom.GetElementByID("task-count")
	if draft, ok, _ := storage.Session().Get("task-draft"); ok {
		taskInput.SetValue(draft)
	}
	showSharedTaskNote()
	taskInput.OnInput(func(event dom.Event) { _ = storage.Session().Set("task-draft", event.Value) })
	dom.GetElementByID("task-form").OnSubmit(func(event dom.Event) {
		event.PreventDefault()
		tasks++
		_ = storage.Session().Remove("task-draft")
		taskInput.SetValue("")
		taskCount.SetText(strconv.Itoa(tasks) + " tasks in this tab")
	})
	storage.OnChange(func(event storage.Event) {
		if event.Key == "workspace-note" {
			showSharedTaskNote()
		}
	})
	_, _ = scheduler.SetInterval(scheduler.Second, func() {
		dom.GetElementByID("status").SetText("timer tick")
	})
}

func showSharedTaskNote() {
	value, ok, _ := storage.Local().Get("workspace-note")
	if !ok || value == "" {
		value = "Waiting for a Storage Event"
	}
	dom.GetElementByID("shared-note").SetText(value)
}
