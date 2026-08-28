package browser

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestKeyboardFocusDefaultActionUpdatesFocusVisibleAndWithin(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", nil)
	button := document.CreateElement("button", nil)
	if err := document.AppendChild(document.Root, form); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(form, button); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`button:focus-visible { color: green } form:focus-within { background-color: blue }`))
	if err != nil {
		t.Fatal(err)
	}
	page := &Page{Document: document, Stylesheet: stylesheet, ComputedStyles: style.Compute(document, stylesheet), Events: events.NewDispatcher()}
	browser := New(nil)
	browser.SetPage(page)
	if !browser.MoveFormFocus(false) || page.FocusTarget != button.ID || !page.FocusVisible {
		t.Fatalf("keyboard focus = target:%d visible:%t", page.FocusTarget, page.FocusVisible)
	}
	buttonStyle, _ := page.ComputedStyles.For(button)
	formStyle, _ := page.ComputedStyles.For(form)
	if buttonStyle.Color != 0x008000ff || formStyle.BackgroundColor != 0x0000ffff {
		t.Fatalf("focus styles = button:%#v form:%#v", buttonStyle, formStyle)
	}
}
