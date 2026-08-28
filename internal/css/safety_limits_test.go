package css

import (
	"fmt"
	"strings"
	"testing"
)

func TestRuleAndSelectorQuotasAreBounded(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(strings.Repeat(".x{}", MaxStylesheetRules+1)))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != MaxStylesheetRules {
		t.Fatalf("rules = %d, want %d", len(stylesheet.Rules), MaxStylesheetRules)
	}

	selectors := make([]string, MaxSelectorsPerRule+1)
	for index := range selectors {
		selectors[index] = fmt.Sprintf(".s%d", index)
	}
	stylesheet, err = Parse(strings.NewReader(strings.Join(selectors, ",") + `{color:red}.ok{color:blue}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 1 || stylesheet.Rules[0].Selectors[0].Class != "ok" {
		t.Fatalf("selector-list recovery = %#v", stylesheet.Rules)
	}

	complex := strings.Repeat("div>", MaxSelectorCombinators+1) + "span"
	stylesheet, err = Parse(strings.NewReader(complex + `{color:red}.after{color:green}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 1 || stylesheet.Rules[0].Selectors[0].Class != "after" {
		t.Fatalf("combinator recovery = %#v", stylesheet.Rules)
	}
}

func TestFunctionalSelectorLayerAndFunctionDepthQuotasAreLocal(t *testing.T) {
	selector := ".target"
	for index := 0; index < MaxFunctionalSelectorDepth+1; index++ {
		selector = ":is(" + selector + ")"
	}
	stylesheet, err := Parse(strings.NewReader(selector + `{color:red}.after{color:green}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 1 || stylesheet.Rules[0].Selectors[0].Class != "after" {
		t.Fatalf("functional selector recovery = %#v", stylesheet.Rules)
	}

	layers := make([]string, MaxCascadeLayers+1)
	for index := range layers {
		layers[index] = fmt.Sprintf("l%d", index)
	}
	stylesheet, err = Parse(strings.NewReader("@layer " + strings.Join(layers, ",") + `; @layer overflow {.bad{color:red}} .after{color:green}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.LayerOrder) != MaxCascadeLayers || len(stylesheet.Rules) != 1 || stylesheet.Rules[0].Selectors[0].Class != "after" {
		t.Fatalf("layer recovery = layers %d rules %#v", len(stylesheet.LayerOrder), stylesheet.Rules)
	}

	tooDeep, allowed := "1px", "1px"
	for index := 0; index < MaxCSSFunctionDepth+1; index++ {
		tooDeep = "calc(" + tooDeep + ")"
	}
	for index := 0; index < MaxCSSFunctionDepth; index++ {
		allowed = "calc(" + allowed + ")"
	}
	stylesheet, err = Parse(strings.NewReader(fmt.Sprintf(`.x{width:%s;color:red}.y{width:%s}`, tooDeep, allowed)))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 2 || len(stylesheet.Rules[0].Declarations) != 1 || stylesheet.Rules[0].Declarations[0].Property != "color" || len(stylesheet.Rules[1].Declarations) != 1 {
		t.Fatalf("function-depth localization = %#v", stylesheet.Rules)
	}
}

func TestCustomPropertySizeAndRegistrationCountAreBounded(t *testing.T) {
	oversized := strings.Repeat("x", MaxCustomPropertyValueBytes+1)
	stylesheet, err := Parse(strings.NewReader(`.x{--oversized:` + oversized + `;color:red}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 1 || len(stylesheet.Rules[0].Declarations) != 1 || stylesheet.Rules[0].Declarations[0].Property != "color" {
		t.Fatalf("oversized custom property recovery = %#v", stylesheet.Rules)
	}

	var source strings.Builder
	for index := 0; index < MaxCustomProperties+1; index++ {
		fmt.Fprintf(&source, "@property --p%d { syntax: '*'; initial-value: 0; inherits: false }", index)
	}
	stylesheet, err = Parse(strings.NewReader(source.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Properties) != MaxCustomProperties {
		t.Fatalf("registered properties = %d, want %d", len(stylesheet.Properties), MaxCustomProperties)
	}
}
