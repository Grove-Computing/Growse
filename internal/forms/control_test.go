package forms

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestEditableTextControlType(t *testing.T) {
	document := dom.NewDocument()
	for _, test := range []struct {
		typeValue string
		want      string
		editable  bool
	}{
		{"", "text", true}, {"PASSWORD", "password", true}, {"email", "email", true},
		{"url", "url", true}, {"number", "number", true}, {"custom", "text", true},
		{"checkbox", "", false}, {"hidden", "", false}, {"date", "", false},
	} {
		attributes := map[string]string{}
		if test.typeValue != "" {
			attributes["type"] = test.typeValue
		}
		node := document.CreateElement("input", attributes)
		got, editable := EditableTextControlType(node)
		if got != test.want || editable != test.editable {
			t.Errorf("type %q = (%q, %v), want (%q, %v)", test.typeValue, got, editable, test.want, test.editable)
		}
	}
}
