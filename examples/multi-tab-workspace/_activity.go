package main

import (
	"growse/dom"
	"growse/fetch"
	"growse/scheduler"
	"growse/storage"
)

func main() {
	showActivityNote()
	storage.OnChange(func(event storage.Event) {
		if event.Key == "workspace-note" {
			showActivityNote()
		}
	})
	fetch.Fetch(fetch.Request{URL: "/login"}, loginLoaded, activityFailed)
	_, _ = scheduler.RequestAnimationFrame(func(timestamp scheduler.Timestamp) {
		_ = timestamp
		dom.GetElementByID("frame-state").SetText("active tab frame committed")
	})
}

func loginLoaded(response fetch.Response) {
	if response.Status < 200 || response.Status >= 300 {
		activityFailed("login failed")
		return
	}
	fetch.Fetch(fetch.Request{URL: "/api/activity"}, activityLoaded, activityFailed)
}

func activityLoaded(response fetch.Response) {
	text, err := response.Text()
	if err != nil {
		activityFailed(err.Error())
		return
	}
	dom.GetElementByID("activity-feed").SetText(text)
	dom.GetElementByID("status").SetText("online")
	dom.GetElementByID("status").SetAttribute("class", "pill ready")
}

func activityFailed(message string) {
	dom.GetElementByID("activity-feed").SetText(message)
	dom.GetElementByID("status").SetText("offline")
	dom.GetElementByID("status").SetAttribute("class", "pill offline")
}

func showActivityNote() {
	value, ok, _ := storage.Local().Get("workspace-note")
	if !ok || value == "" {
		value = "Waiting for shared note"
	}
	dom.GetElementByID("shared-note").SetText(value)
}
