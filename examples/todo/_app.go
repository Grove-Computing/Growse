package main

import (
	"growse/dom"
	"growse/strconv"
)

func main() {
	form := dom.GetElementByID("todo-form")
	input := dom.GetElementByID("todo-input")
	list := dom.GetElementByID("todo-list")
	if form == nil || input == nil || list == nil {
		return
	}

	nextID := 0
	form.OnSubmit(func(event dom.Event) {
		_ = event
		value := input.Value()
		if value == "" {
			return
		}

		nextID++
		suffix := strconv.Itoa(nextID)
		item := dom.CreateElement("li")
		item.SetAttribute("id", "todo-"+suffix)
		item.AddClass("todo")
		if !list.AppendChild(item) {
			return
		}

		label := dom.CreateElement("span")
		label.SetText(value)
		item.AppendChild(label)

		toggle := dom.CreateElement("button")
		toggle.SetAttribute("id", "toggle-"+suffix)
		toggle.SetAttribute("type", "button")
		toggle.SetText("Complete")
		item.AppendChild(toggle)

		remove := dom.CreateElement("button")
		remove.SetAttribute("id", "delete-"+suffix)
		remove.SetAttribute("type", "button")
		remove.SetText("Delete")
		item.AppendChild(remove)

		completed := false
		toggle.OnClick(func() {
			if completed {
				item.RemoveClass("completed")
			} else {
				item.AddClass("completed")
			}
			completed = !completed
		})
		remove.OnClick(func() {
			item.Remove()
		})
		input.SetValue("")
	})
}
