package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestHTMLConversionRejectsPageDOMLimits(t *testing.T) {
	attributes := make([]string, dom.MaxAttributesPerNode+1)
	for index := range attributes {
		attributes[index] = fmt.Sprintf(`data-%d="x"`, index)
	}
	if _, err := Parse(strings.NewReader("<div " + strings.Join(attributes, " ") + "></div>")); err == nil {
		t.Fatal("HTML attribute limit error = nil")
	}
	if _, err := Parse(strings.NewReader("<p>" + strings.Repeat("x", dom.MaxDOMStringBytes+1) + "</p>")); err == nil {
		t.Fatal("HTML string limit error = nil")
	}
	deep := strings.Repeat("<div>", dom.MaxTreeDepth+1) + strings.Repeat("</div>", dom.MaxTreeDepth+1)
	if _, err := Parse(strings.NewReader(deep)); err == nil {
		t.Fatal("HTML depth limit error = nil")
	}
	if _, err := Parse(strings.NewReader(strings.Repeat("<i></i>", dom.MaxNodesPerDocument))); err == nil {
		t.Fatal("HTML node count limit error = nil")
	}
}
