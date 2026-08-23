package main

import (
	"growse/dom"
	"growse/fetch"
)

var status *dom.Element
var items *dom.Element
var controller *fetch.AbortController

func main() {
	status = dom.GetElementByID("status")
	items = dom.GetElementByID("items")
	if form := dom.GetElementByID("item-form"); form != nil {
		form.OnSubmit(cancelNativeSubmit)
	}
	if cancel := dom.GetElementByID("cancel"); cancel != nil {
		cancel.OnClick(cancelFetch)
	}
	load()
}

func load() {
	controller = fetch.NewAbortController()
	headers := fetch.NewHeaders()
	if err := headers.Append("Accept", "application/json"); err != nil {
		failed(err.Error())
		return
	}
	fetch.Fetch(fetch.Request{URL: "/api/items", Headers: headers, Signal: controller.Signal(), Timeout: 5 * fetch.Second}, loaded, failed)
}

func cancelFetch() {
	if controller != nil {
		controller.Abort()
		status.SetText("cancel requested")
	}
}

func cancelNativeSubmit(event dom.Event) {
	event.PreventDefault()
}

func loaded(response fetch.Response) {
	text, err := response.Text()
	if err != nil {
		failed(err.Error())
		return
	}
	items.SetAttribute("class", "success")
	items.SetText(text)
	status.SetAttribute("class", "success")
	status.SetText("success")
}

func failed(message string) {
	status.SetAttribute("class", "network-error")
	status.SetText("network error: " + message)
}
