package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestNamedContainerQueryAndContainerUnitsUseMeasuredAncestor(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("section", map[string]string{"class": "container"})
	child := document.CreateElement("div", map[string]string{"class": "child"})
	appendNode(t, document, document.Root, container)
	appendNode(t, document, container, child)
	stylesheet, err := css.Parse(strings.NewReader(`
.container { container-type: inline-size; container-name: card; width: 400px }
.child { width: 25cqw; color: red }
@container card (min-width: 350px) and (max-width: 450px) {
  .child { width: 50cqw; color: green }
}
@container other (min-width: 1px) { .child { color: blue } }
`))
	if err != nil {
		t.Fatal(err)
	}
	environment := Environment{
		ViewportWidth: 800, ViewportHeight: 600, RootFontSize: 16, ResolutionDPI: 96,
		ContainerSizes: map[dom.NodeID]ContainerSize{container.ID: {Width: 400, Height: 200}},
	}
	computed := ComputeWithEnvironment(document, stylesheet, InteractionState{}, environment)
	containerStyle, _ := computed.For(container)
	childStyle, _ := computed.For(child)
	if containerStyle.ContainerType != ContainerTypeInlineSize || containerStyle.ContainerName != "card" {
		t.Fatalf("container style = %#v", containerStyle)
	}
	if childStyle.Color != 0x008000ff || childStyle.Width.Kind != SizeLength || childStyle.Width.Value.Pixels != 200 {
		t.Fatalf("queried child style = %#v", childStyle)
	}
}

func TestContainerUnitsFallBackToViewportWithoutEligibleContainer(t *testing.T) {
	length, ok := ResolveLength("25cqw", LengthContext{ViewportWidth: 800, ViewportHeight: 600})
	if !ok || length.Pixels != 200 {
		t.Fatalf("25cqw fallback = %#v, valid %v", length, ok)
	}
}
