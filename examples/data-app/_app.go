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
	headers := fetch.NewHeaders()
	if err := headers.Append("Accept", "application/json"); err != nil {
		failed(err.Error())
		return
	}
	fetch.Fetch(fetch.Request{URL: "/api/items", Headers: headers}, loaded, failed)
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
