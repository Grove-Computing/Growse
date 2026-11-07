package main

import "growse/dom"

func main() {
	button := dom.GetElementByID("mutate-find")
	target := dom.GetElementByID("mutation-target")
	state := dom.GetElementByID("mutation-state")
	if button == nil || target == nil || state == nil {
		return
	}
	button.OnClick(func() {
		target.SetText("mutated needle · 動的な検索対象 · Cafe\u0301")
		state.SetText("mutation committed")
	})
}
