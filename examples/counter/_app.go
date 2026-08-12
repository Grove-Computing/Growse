package main

import (
	"growse/dom"
	"growse/strconv"
)

func main() {
	button := dom.GetElementByID("increment")
	output := dom.GetElementByID("count")
	if button == nil || output == nil {
		return
	}

	count := 0
	button.OnClick(func() {
		count++
		output.SetText(strconv.Itoa(count))
	})
}
