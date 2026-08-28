package style

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestCustomPropertyCountIsBoundedWithoutDroppingNormalDeclarations(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", nil)
	appendNode(t, document, document.Root, target)
	declarations := make([]css.Declaration, 0, css.MaxCustomProperties+2)
	for index := 0; index < css.MaxCustomProperties+1; index++ {
		declarations = append(declarations, css.Declaration{Property: fmt.Sprintf("--p%d", index), Value: css.Value{Raw: "1px"}})
	}
	declarations = append(declarations, css.Declaration{Property: "color", Value: css.Value{Raw: "red"}})
	stylesheet := &css.Stylesheet{Rules: []css.Rule{{Selectors: css.ParseSelectorList("*"), Declarations: declarations}}}
	computed, _ := Compute(document, stylesheet).For(target)
	if len(computed.CustomProperties) != css.MaxCustomProperties || computed.Color != 0xff0000ff {
		t.Fatalf("bounded custom properties = %d, color %08x", len(computed.CustomProperties), computed.Color)
	}
}

func TestVarCalcAndColorDepthFailuresStayLocal(t *testing.T) {
	properties := make(map[string]string, css.MaxCSSFunctionDepth+1)
	for index := 0; index < css.MaxCSSFunctionDepth+1; index++ {
		name := fmt.Sprintf("--p%d", index)
		next := "1px"
		if index < css.MaxCSSFunctionDepth {
			next = fmt.Sprintf("var(--p%d)", index+1)
		}
		properties[name] = next
	}
	if _, valid := resolveVariables("var(--p0)", properties); valid {
		t.Fatal("over-depth var chain resolved")
	}
	properties[fmt.Sprintf("--p%d", css.MaxCSSFunctionDepth-1)] = "1px"
	if resolved, valid := resolveVariables("var(--p0)", properties); !valid || resolved != "1px" {
		t.Fatalf("bounded var chain = %q, %v", resolved, valid)
	}

	tooDeepCalc := "1px"
	for index := 0; index < css.MaxCSSFunctionDepth+1; index++ {
		tooDeepCalc = "calc(" + tooDeepCalc + ")"
	}
	if _, valid := ResolveLength(tooDeepCalc, LengthContext{FontSize: 16, RootFontSize: 16}); valid {
		t.Fatal("over-depth calc resolved")
	}

	tooDeepColor := "red"
	for index := 0; index < css.MaxCSSFunctionDepth+1; index++ {
		tooDeepColor = "color-mix(in srgb, blue, " + tooDeepColor + ")"
	}
	if _, valid := parseColor(tooDeepColor, 0); valid {
		t.Fatal("over-depth color resolved")
	}

	document := dom.NewDocument()
	target := document.CreateElement("div", map[string]string{"style": "width:" + tooDeepCalc + "; color:green"})
	appendNode(t, document, document.Root, target)
	computed, _ := Compute(document, nil).For(target)
	if computed.Width.Kind != SizeAuto || computed.Color != 0x008000ff {
		t.Fatalf("localized computed values = %#v", computed)
	}
}

func TestVariableExpansionCannotExceedValueQuota(t *testing.T) {
	chunk := strings.Repeat("x", css.MaxCustomPropertyValueBytes/2+1)
	if _, valid := resolveVariables("var(--a)var(--a)", map[string]string{"--a": chunk}); valid {
		t.Fatal("oversized var expansion resolved")
	}
}
