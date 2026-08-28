package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestAdditionalFormAndFocusPseudoClassesFollowDOMState(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", map[string]string{"class": "form"})
	empty := document.CreateElement("input", map[string]string{"class": "empty", "placeholder": "Name", "required": ""})
	readonly := document.CreateElement("input", map[string]string{"class": "locked", "readonly": ""})
	optional := document.CreateElement("textarea", map[string]string{"class": "optional"})
	custom := document.CreateElement("x-widget", map[string]string{"class": "custom"})
	editable := document.CreateElement("div", map[string]string{"class": "editable", "contenteditable": "true"})
	for _, edge := range [][2]*dom.Node{{document.Root, form}, {form, empty}, {form, readonly}, {form, optional}, {form, custom}, {form, editable}} {
		appendNode(t, document, edge[0], edge[1])
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.empty:placeholder-shown:required:read-write { color: green }
.locked:read-only { color: blue }
.optional:optional { background-color: blue }
.form:focus-within { font-size: 20px }
.empty:focus-visible { width: 100px }
.custom:defined { color: red }
.editable:read-write { height: 30px }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := ComputeWithState(document, stylesheet, InteractionState{Focused: empty.ID, FocusVisible: true})
	emptyStyle, _ := computed.For(empty)
	lockedStyle, _ := computed.For(readonly)
	optionalStyle, _ := computed.For(optional)
	formStyle, _ := computed.For(form)
	customStyle, _ := computed.For(custom)
	editableStyle, _ := computed.For(editable)
	if emptyStyle.Color != 0x008000ff || emptyStyle.Width.Value.Pixels != 100 || lockedStyle.Color != 0x0000ffff || optionalStyle.BackgroundColor != 0x0000ffff {
		t.Fatalf("form pseudo styles = empty:%#v locked:%#v optional:%#v", emptyStyle, lockedStyle, optionalStyle)
	}
	if formStyle.FontSize != 20 || customStyle.Color == 0xff0000ff || editableStyle.Height.Value.Pixels != 30 {
		t.Fatalf("focus/defined/read-write styles = form:%#v custom:%#v editable:%#v", formStyle, customStyle, editableStyle)
	}
}
