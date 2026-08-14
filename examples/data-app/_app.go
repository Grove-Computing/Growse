package main

import (
	"growse/dom"
	"growse/fetch"
)

var status *dom.Element
var items *dom.Element

func main() {
	status = dom.GetElementByID("status")
	items = dom.GetElementByID("items")
	if form := dom.GetElementByID("item-form"); form != nil {
		form.OnSubmit(cancelNativeSubmit)
	}
	fetch.Fetch(fetch.Request{URL: "/api/items"}, loaded, failed)
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
