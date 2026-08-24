package main

import (
	"growse/console"
	"growse/dom"
	"growse/fetch"
)

func main() {
	console.Log("DevTools showcase started")
	console.Info("Console info fixture")
	console.Warn("Console warning fixture")
	console.Error("Console error fixture")
	setState("console-state", "four levels emitted", "ready")
	setState("mutation-state", "mutated by WebGo", "ready")

	headers := fetch.NewHeaders()
	_ = headers.Append("X-API-Key", "showcase-api-secret")
	fetch.Fetch(fetch.Request{URL: "/api/success?token=showcase-query-secret", Headers: headers}, func(fetch.Response) {
		setState("success-state", "success", "ready")
	}, func(string) { setState("success-state", "failed", "failed") })
	fetch.Fetch(fetch.Request{URL: "/api/redirect"}, func(fetch.Response) {
		setState("redirect-state", "redirect followed", "ready")
	}, func(string) { setState("redirect-state", "failed", "failed") })
	fetch.Fetch(fetch.Request{URL: "/api/error"}, func(response fetch.Response) {
		if response.Status == 503 {
			setState("error-state", "HTTP 503", "failed")
		}
	}, func(string) { setState("error-state", "failed", "failed") })
	fetch.Fetch(fetch.Request{URL: "/api/slow", Timeout: 20 * fetch.Millisecond}, func(fetch.Response) {
		setState("timeout-state", "unexpected success", "failed")
	}, func(string) { setState("timeout-state", "timeout", "failed") })
	fetch.Fetch(fetch.Request{URL: "/api/cache"}, cacheFirst, func(string) {
		setState("cache-state", "failed", "failed")
	})
}

func cacheFirst(response fetch.Response) {
	_ = response
	fetch.Fetch(fetch.Request{URL: "/api/cache"}, func(fetch.Response) {
		setState("cache-state", "miss then hit", "ready")
	}, func(string) { setState("cache-state", "failed", "failed") })
}

func setState(id, value, className string) {
	element := dom.GetElementByID(id)
	if element == nil {
		return
	}
	element.SetText(value)
	element.SetAttribute("class", className)
}
