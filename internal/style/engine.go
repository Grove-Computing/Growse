package style

import (
	"strconv"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/forms"
)

const (
	defaultTextColor = uint32(0x202124ff)
	transparent      = uint32(0x00000000)
)

type winner struct {
	value       string
	source      string
	important   bool
	inline      bool
	specificity [3]int
	order       [2]int
	layer       string
	layerOrder  int
}

// Compute applies UA defaults, inheritance, selector matching and cascade.
func Compute(document *dom.Document, stylesheet *css.Stylesheet) Map {
	return ComputeWithState(document, stylesheet, InteractionState{})
}

// Environment contains rendering metrics needed during value computation.
type Environment struct {
	ViewportWidth   float32
	ViewportHeight  float32
	RootFontSize    float32
	ResolutionDPI   float32
	ColorScheme     string
	Hover           bool
	Pointer         string
	ReducedMotion   bool
	ContainerSizes  map[dom.NodeID]ContainerSize
	ContainerWidth  float32
	ContainerHeight float32
}

// ContainerSize is the previous layout iteration's content-box size.
type ContainerSize struct{ Width, Height float32 }

func defaultEnvironment() Environment {
	return Environment{
		ViewportWidth: 1280, ViewportHeight: 720, RootFontSize: 16,
		ResolutionDPI: 96, ColorScheme: "light", Hover: true, Pointer: "fine",
	}
}

// ComputeWithState applies styles using transient browser interaction state.
func ComputeWithState(document *dom.Document, stylesheet *css.Stylesheet, state InteractionState) Map {
	return ComputeWithEnvironment(document, stylesheet, state, defaultEnvironment())
}

// ComputeWithEnvironment applies styles using interaction and viewport state.
func ComputeWithEnvironment(document *dom.Document, stylesheet *css.Stylesheet, state InteractionState, environment Environment) Map {
	result := make(Map)
	if document == nil || document.Root == nil {
		return result
	}
	state.Document = document
	if environment.ViewportWidth <= 0 {
		environment.ViewportWidth = defaultEnvironment().ViewportWidth
	}
	if environment.ViewportHeight <= 0 {
		environment.ViewportHeight = defaultEnvironment().ViewportHeight
	}
	if environment.RootFontSize <= 0 {
		environment.RootFontSize = 16
	}
	if environment.ResolutionDPI <= 0 {
		environment.ResolutionDPI = 96
	}
	if environment.ColorScheme == "" {
		environment.ColorScheme = "light"
	}
	if environment.Pointer == "" {
		environment.Pointer = "fine"
	}
	computeNode(document.Root, initialStyle(), stylesheet, state, environment, result)
	return result
}

func computeNode(node *dom.Node, parent ComputedStyle, stylesheet *css.Stylesheet, state InteractionState, environment Environment, result Map) {
	if node == nil || node.Type == dom.NodeDocumentFragment {
		return
	}
	computed := inheritedStyle(parent)
	if node.Type == dom.NodeDocument {
		computed = initialStyle()
	} else if node.Type == dom.NodeElement {
		computed = applyUADefaults(node.TagName, computed)
		computed = applyPresentationalHints(node, computed)
		computed = applyAuthorRules(node, computed, parent, stylesheet, state, environment, result)
		computed = applyGeneratedContent(node, computed, stylesheet, state, environment, result)
		result[node.ID] = computed
	} else if node.Type == dom.NodeText {
		result[node.ID] = computed
	}

	childEnvironment := environment
	if node.Type == dom.NodeElement && node.TagName == "html" {
		childEnvironment.RootFontSize = computed.FontSize
	}
	if node.Type == dom.NodeElement && computed.ContainerType == ContainerTypeInlineSize {
		if size, ok := environment.ContainerSizes[node.ID]; ok {
			childEnvironment.ContainerWidth, childEnvironment.ContainerHeight = size.Width, size.Height
		}
	}
	for _, child := range node.Children {
		computeNode(child, computed, stylesheet, state, childEnvironment, result)
	}
}

func applyPresentationalHints(node *dom.Node, computed ComputedStyle) ComputedStyle {
	if node == nil || node.TagName != "iframe" {
		return computed
	}
	for name, target := range map[string]*SizeValue{"width": &computed.Width, "height": &computed.Height} {
		value, ok := node.Attribute(name)
		if !ok {
			continue
		}
		pixels, err := strconv.ParseFloat(strings.TrimSpace(value), 32)
		if err == nil && pixels > 0 && pixels <= 16_384 {
			*target = SizeValue{Kind: SizeLength, Value: LengthPercentage{Pixels: float32(pixels)}}
		}
	}
	return computed
}

func initialStyle() ComputedStyle {
	return ComputedStyle{
		Color: defaultTextColor, BackgroundColor: transparent, FontSize: 16, FontWeight: 400,
		FontFamilies: []string{"Growse Sans", "sans-serif"}, FontStyle: "normal", FontStretch: "normal", FontFaceIndex: -1,
		BackgroundRepeat: BackgroundRepeat{X: true, Y: true},
		DecorationColor:  defaultTextColor, Opacity: 1, FlexShrink: 1,
		ZIndexAuto: true,
		AlignItems: AlignStretch, JustifyItems: AlignStretch, AlignContent: AlignStretch, AlignSelf: AlignAuto, JustifySelf: AlignAuto,
		Width: SizeValue{Kind: SizeAuto}, Height: SizeValue{Kind: SizeAuto},
		MinWidth: SizeValue{Kind: SizeAuto}, MinHeight: SizeValue{Kind: SizeAuto},
		MaxWidth: SizeValue{Kind: SizeNone}, MaxHeight: SizeValue{Kind: SizeNone},
		Transitions: defaultTransitions(),
		Animations:  defaultAnimations(),
	}
}

func inheritedStyle(parent ComputedStyle) ComputedStyle {
	computed := ComputedStyle{
		Color: parent.Color, FontSize: parent.FontSize, FontWeight: parent.FontWeight,
		FontFamilies: append([]string(nil), parent.FontFamilies...), FontStyle: parent.FontStyle, FontStretch: parent.FontStretch, FontFaceIndex: parent.FontFaceIndex,
		LineHeight: parent.LineHeight, WhiteSpace: parent.WhiteSpace, Visibility: parent.Visibility,
		BackgroundColor: transparent, Display: DisplayInline,
		BackgroundRepeat: BackgroundRepeat{X: true, Y: true},
		DecorationColor:  parent.Color, Opacity: 1, FlexShrink: 1,
		ZIndexAuto: true,
		AlignItems: AlignStretch, JustifyItems: AlignStretch, AlignContent: AlignStretch, AlignSelf: AlignAuto, JustifySelf: AlignAuto,
		Width: SizeValue{Kind: SizeAuto}, Height: SizeValue{Kind: SizeAuto},
		MinWidth: SizeValue{Kind: SizeAuto}, MinHeight: SizeValue{Kind: SizeAuto},
		MaxWidth: SizeValue{Kind: SizeNone}, MaxHeight: SizeValue{Kind: SizeNone},
		Transitions: defaultTransitions(),
		Animations:  defaultAnimations(),
	}
	if len(parent.CustomProperties) != 0 {
		computed.CustomProperties = make(map[string]string, len(parent.CustomProperties))
		for name, value := range parent.CustomProperties {
			computed.CustomProperties[name] = value
		}
	}
	return computed
}

func applyUADefaults(tag string, computed ComputedStyle) ComputedStyle {
	switch tag {
	case "html", "body", "div", "main", "section", "article", "header", "footer", "nav", "form", "ul", "ol", "input":
		computed.Display = DisplayBlock
	case "h1":
		computed.Display, computed.FontSize, computed.FontWeight = DisplayBlock, 32, 700
		computed.Margin = Edges{Top: 12, Bottom: 12}
	case "h2":
		computed.Display, computed.FontSize, computed.FontWeight = DisplayBlock, 26, 700
		computed.Margin = Edges{Top: 10, Bottom: 10}
	case "h3":
		computed.Display, computed.FontSize, computed.FontWeight = DisplayBlock, 21, 700
		computed.Margin = Edges{Top: 8, Bottom: 8}
	case "h4", "h5", "h6":
		computed.Display, computed.FontWeight = DisplayBlock, 700
		computed.Margin = Edges{Top: 8, Bottom: 8}
	case "p":
		computed.Display = DisplayBlock
		computed.Margin.Bottom = 14
	case "button":
		computed.FontWeight = 700
	case "iframe":
		computed.Display = DisplayInlineBlock
		computed.Width = SizeValue{Kind: SizeLength, Value: LengthPercentage{Pixels: 300}}
		computed.Height = SizeValue{Kind: SizeLength, Value: LengthPercentage{Pixels: 150}}
	case "a":
		computed.Color = 0x0969daff
	case "pre":
		computed.Display, computed.FontSize = DisplayBlock, 15
		computed.Margin.Bottom = 14
	case "li":
		computed.Display = DisplayBlock
		computed.Margin.Bottom = 6
	case "head", "script", "style", "noscript", "template":
		computed.Display = DisplayNone
	}
	return computed
}

func applyAuthorRules(node *dom.Node, computed, parent ComputedStyle, stylesheet *css.Stylesheet, state InteractionState, environment Environment, computedAncestors Map) ComputedStyle {
	if stylesheet == nil {
		stylesheet = &css.Stylesheet{}
	}
	layerOrders := make(map[string]int, len(stylesheet.LayerOrder))
	for index, layer := range stylesheet.LayerOrder {
		layerOrders[layer] = index
	}
	candidates := make(map[string][]winner)
	for _, rule := range stylesheet.Rules {
		if !matchesMediaGroups(rule.Media, environment) || !matchesSupportsGroups(rule.Supports) || !matchesContainerGroups(node, rule.Containers, computedAncestors, environment) {
			continue
		}
		for _, selector := range rule.Selectors {
			if selectorPseudoElement(selector) != css.PseudoElementNone {
				continue
			}
			if !matches(node, selector, state) {
				continue
			}
			for declarationIndex, declaration := range rule.Declarations {
				for _, property := range expandedProperties(declaration.Property) {
					candidate := winner{
						value: declaration.Value.Raw, source: declaration.Property, important: declaration.Important,
						specificity: selector.Specificity(), order: [2]int{rule.Order, declarationIndex}, layer: rule.Layer,
					}
					if rule.Layer != "" {
						candidate.layerOrder = layerOrders[rule.Layer]
					}
					candidates[property] = append(candidates[property], candidate)
				}
			}
		}
	}
	if inlineValue, ok := node.Attribute("style"); ok {
		declarations, _ := css.ParseDeclarations(inlineValue)
		for declarationIndex, declaration := range declarations {
			for _, property := range expandedProperties(declaration.Property) {
				candidate := winner{
					value: declaration.Value.Raw, source: declaration.Property, important: declaration.Important, inline: true,
					order: [2]int{0, declarationIndex},
				}
				candidates[property] = append(candidates[property], candidate)
			}
		}
	}
	winners := make(map[string]winner, len(candidates))
	for property, propertyCandidates := range candidates {
		if selected, ok := selectCascadeWinner(propertyCandidates); ok {
			winners[property] = selected
		}
	}
	for property, candidate := range winners {
		if !candidate.important {
			continue
		}
		if computed.ImportantProperties == nil {
			computed.ImportantProperties = make(map[string]bool)
		}
		computed.ImportantProperties[property] = true
	}
	propertyContext := LengthContext{FontSize: parent.FontSize, RootFontSize: environment.RootFontSize, ViewportWidth: environment.ViewportWidth, ViewportHeight: environment.ViewportHeight, ContainerWidth: environment.ContainerWidth, ContainerHeight: environment.ContainerHeight}
	computed.CustomProperties = registeredPropertyBase(parent.CustomProperties, stylesheet, propertyContext)
	computed.CustomProperties = applyCustomProperties(computed.CustomProperties, winners, stylesheet, propertyContext, parent.CustomProperties)
	fontContext := LengthContext{
		FontSize: parent.FontSize, RootFontSize: environment.RootFontSize,
		ViewportWidth: environment.ViewportWidth, ViewportHeight: environment.ViewportHeight,
		PercentageBase: parent.FontSize,
		ContainerWidth: environment.ContainerWidth, ContainerHeight: environment.ContainerHeight,
	}

	if value, ok := winners["color"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveColor(resolved, parent.Color, defaultTextColor, true, parent.Color); valid {
				computed.Color = parsed
			}
		}
	}
	if value, ok := winners["background-color"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveColor(resolved, parent.BackgroundColor, transparent, false, computed.Color); valid {
				computed.BackgroundColor = parsed
			}
		}
	}
	if value, ok := winners["text-decoration-line"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			switch parseGlobalKeyword(resolved) {
			case globalInherit:
				computed.TextDecoration = parent.TextDecoration
			case globalInitial, globalUnset:
				computed.TextDecoration = TextDecorationNone
			default:
				if parsed, valid := parseTextDecorationLine(resolved); valid {
					computed.TextDecoration = parsed
				}
			}
		}
	}
	if value, ok := winners["text-decoration-color"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveColor(resolved, parent.DecorationColor, computed.Color, false, computed.Color); valid {
				computed.DecorationColor = parsed
			}
		}
	} else {
		computed.DecorationColor = computed.Color
	}
	if value, ok := winners["opacity"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveFloat(resolved, parent.Opacity, 1, false, parseOpacity); valid {
				computed.Opacity = parsed
			}
		}
	}
	if value, ok := winners["background-image"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			switch parseGlobalKeyword(resolved) {
			case globalInherit:
				computed.BackgroundImage = parent.BackgroundImage
			case globalInitial, globalUnset:
				computed.BackgroundImage = BackgroundImage{}
			default:
				if parsed, valid := parseBackgroundImage(resolved, computed.Color); valid {
					computed.BackgroundImage = parsed
				}
			}
		}
	}
	if value, ok := winners["background-repeat"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			switch parseGlobalKeyword(resolved) {
			case globalInherit:
				computed.BackgroundRepeat = parent.BackgroundRepeat
			case globalInitial, globalUnset:
				computed.BackgroundRepeat = BackgroundRepeat{X: true, Y: true}
			default:
				if parsed, valid := parseBackgroundRepeat(resolved); valid {
					computed.BackgroundRepeat = parsed
				}
			}
		}
	}
	if value, ok := winners["background-position"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			switch parseGlobalKeyword(resolved) {
			case globalInherit:
				computed.BackgroundPos = parent.BackgroundPos
			case globalInitial, globalUnset:
				computed.BackgroundPos = BackgroundPosition{}
			default:
				if parsed, valid := parseBackgroundPosition(resolved, fontContext); valid {
					computed.BackgroundPos = parsed
				}
			}
		}
	}
	if value, ok := winners["background-size"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			switch parseGlobalKeyword(resolved) {
			case globalInherit:
				computed.BackgroundSize = parent.BackgroundSize
			case globalInitial, globalUnset:
				computed.BackgroundSize = BackgroundSize{}
			default:
				if parsed, valid := parseBackgroundSize(resolved, fontContext); valid {
					computed.BackgroundSize = parsed
				}
			}
		}
	}
	computed = applyBackgroundLayers(computed, parent, winners, computed.CustomProperties, fontContext)
	if value, ok := winners["font-size"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			parseFontSize := func(value string) (float32, bool) {
				length, valid := ResolveLength(value, fontContext)
				resolved := length.Resolve(fontContext.PercentageBase)
				return resolved, valid && resolved > 0
			}
			if parsed, valid := resolveFloat(resolved, parent.FontSize, 16, true, parseFontSize); valid {
				computed.FontSize = parsed
			}
		}
	}
	if value, ok := winners["font-family"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := parseFontFamilies(resolved); valid {
				computed.FontFamilies = parsed
			}
		}
	}
	if value, ok := winners["font-style"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := parseFontStyle(resolved); valid {
				computed.FontStyle = parsed
			}
		}
	}
	if value, ok := winners["font-stretch"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := parseFontStretch(resolved); valid {
				computed.FontStretch = parsed
			}
		}
	}
	if value, ok := winners["font-weight"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveInt(resolved, parent.FontWeight, 400, true, parseFontWeight); valid {
				computed.FontWeight = parsed
			}
		}
	}
	computed.FontFaceIndex = selectFontFace(stylesheet, computed.FontFamilies, computed.FontStyle, computed.FontWeight, computed.FontStretch)
	if value, ok := winners["line-height"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveLineHeight(resolved, parent.LineHeight, computed.FontSize, fontContext); valid {
				computed.LineHeight = parsed
			}
		}
	}
	if value, ok := winners["white-space"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveWhiteSpace(resolved, parent.WhiteSpace); valid {
				computed.WhiteSpace = parsed
			}
		}
	}
	if value, ok := winners["overflow-x"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveOverflow(resolved, parent.OverflowX); valid {
				computed.OverflowX = parsed
			}
		}
	}
	if value, ok := winners["overflow-y"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveOverflow(resolved, parent.OverflowY); valid {
				computed.OverflowY = parsed
			}
		}
	}
	if value, ok := winners["display"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveDisplay(resolved, parent.Display); valid {
				computed.Display = parsed
			}
		}
	}
	if value, ok := winners["visibility"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			switch parseGlobalKeyword(resolved) {
			case globalInherit, globalUnset:
				computed.Visibility = parent.Visibility
			case globalInitial:
				computed.Visibility = VisibilityVisible
			default:
				switch strings.ToLower(strings.TrimSpace(resolved)) {
				case "visible":
					computed.Visibility = VisibilityVisible
				case "hidden", "collapse":
					computed.Visibility = VisibilityHidden
				}
			}
		}
	}
	if value, ok := winners["box-sizing"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			if parsed, valid := resolveBoxSizing(resolved, parent.BoxSizing); valid {
				computed.BoxSizing = parsed
			}
		}
	}
	if value, ok := winners["container-type"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			switch strings.ToLower(strings.TrimSpace(resolved)) {
			case "inline-size":
				computed.ContainerType = ContainerTypeInlineSize
			case "normal", "initial", "unset":
				computed.ContainerType = ContainerTypeNormal
			}
		}
	}
	if value, ok := winners["container-name"]; ok {
		if resolved, ok := resolveVariables(value.value, computed.CustomProperties); ok {
			name := strings.TrimSpace(resolved)
			if name == "none" || name == "initial" || name == "unset" {
				computed.ContainerName = ""
			} else if !strings.ContainsAny(name, " \t\r\n/()") {
				computed.ContainerName = name
			}
		}
	}
	lengthContext := LengthContext{
		FontSize: computed.FontSize, RootFontSize: environment.RootFontSize,
		ViewportWidth: environment.ViewportWidth, ViewportHeight: environment.ViewportHeight,
		PercentageBase: environment.ViewportWidth,
		ContainerWidth: environment.ContainerWidth, ContainerHeight: environment.ContainerHeight,
	}
	computed.Width = resolveSizeWinner("width", computed.Width, parent.Width, winners, computed.CustomProperties, lengthContext)
	computed.Height = resolveSizeWinner("height", computed.Height, parent.Height, winners, computed.CustomProperties, lengthContext)
	computed.MinWidth = resolveSizeWinner("min-width", computed.MinWidth, parent.MinWidth, winners, computed.CustomProperties, lengthContext)
	computed.MinHeight = resolveSizeWinner("min-height", computed.MinHeight, parent.MinHeight, winners, computed.CustomProperties, lengthContext)
	computed.MaxWidth = resolveSizeWinner("max-width", computed.MaxWidth, parent.MaxWidth, winners, computed.CustomProperties, lengthContext)
	computed.MaxHeight = resolveSizeWinner("max-height", computed.MaxHeight, parent.MaxHeight, winners, computed.CustomProperties, lengthContext)
	computed.Margin, computed.MarginAuto = applyMargins(computed.Margin, computed.MarginAuto, parent.Margin, parent.MarginAuto, winners, computed.CustomProperties, lengthContext)
	computed.Padding = applyEdges(computed.Padding, parent.Padding, "padding", winners, computed.CustomProperties, lengthContext)
	computed.Border = applyBorders(computed.Border, parent.Border, winners, computed.CustomProperties, lengthContext, computed.Color)
	computed.BorderRadius = applyBorderRadii(computed.BorderRadius, parent.BorderRadius, winners, computed.CustomProperties, lengthContext)
	computed = applyFlexProperties(computed, parent, winners, computed.CustomProperties, lengthContext)
	computed = applyGridProperties(computed, parent, winners, computed.CustomProperties, lengthContext)
	computed = applyPositionProperties(computed, parent, winners, computed.CustomProperties, lengthContext)
	computed = applyShadowAndOutlineProperties(computed, parent, winners, computed.CustomProperties, lengthContext)
	computed = applyTransformProperties(computed, parent, winners, computed.CustomProperties, lengthContext)
	computed = applyTransitionProperties(computed, parent, winners, computed.CustomProperties)
	computed = applyAnimationProperties(computed, parent, winners, computed.CustomProperties)
	return computed
}

type globalKeyword uint8

const (
	globalNone globalKeyword = iota
	globalInherit
	globalInitial
	globalUnset
)

func parseGlobalKeyword(value string) globalKeyword {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "inherit":
		return globalInherit
	case "initial":
		return globalInitial
	case "unset":
		return globalUnset
	default:
		return globalNone
	}
}

func resolveColor(value string, parent, initial uint32, inherited bool, currentColor uint32) (uint32, bool) {
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent, true
	case globalInitial:
		return initial, true
	case globalUnset:
		if inherited {
			return parent, true
		}
		return initial, true
	default:
		return parseColor(value, currentColor)
	}
}

func resolveFloat(value string, parent, initial float32, inherited bool, parse func(string) (float32, bool)) (float32, bool) {
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent, true
	case globalInitial:
		return initial, true
	case globalUnset:
		if inherited {
			return parent, true
		}
		return initial, true
	default:
		return parse(value)
	}
}

func resolveInt(value string, parent, initial int, inherited bool, parse func(string) (int, bool)) (int, bool) {
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent, true
	case globalInitial:
		return initial, true
	case globalUnset:
		if inherited {
			return parent, true
		}
		return initial, true
	default:
		return parse(value)
	}
}

func resolveDisplay(value string, parent Display) (Display, bool) {
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent, true
	case globalInitial, globalUnset:
		return DisplayInline, true
	default:
		return parseDisplay(value)
	}
}

func resolveBoxSizing(value string, parent BoxSizing) (BoxSizing, bool) {
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent, true
	case globalInitial, globalUnset:
		return BoxSizingContentBox, true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "content-box":
		return BoxSizingContentBox, true
	case "border-box":
		return BoxSizingBorderBox, true
	default:
		return 0, false
	}
}

func resolveSizeWinner(property string, current, parent SizeValue, winners map[string]winner, customProperties map[string]string, context LengthContext) SizeValue {
	candidate, ok := winners[property]
	if !ok {
		return current
	}
	resolved, ok := resolveVariables(candidate.value, customProperties)
	if !ok {
		return current
	}
	switch parseGlobalKeyword(resolved) {
	case globalInherit:
		return parent
	case globalInitial, globalUnset:
		return initialSizeValue(property)
	}
	value := strings.ToLower(strings.TrimSpace(resolved))
	if (property == "width" || property == "height" || property == "min-width" || property == "min-height") && value == "auto" {
		return SizeValue{Kind: SizeAuto}
	}
	if strings.HasPrefix(property, "max-") && value == "none" {
		return SizeValue{Kind: SizeNone}
	}
	length, valid := ResolveLength(value, context)
	if !valid || length.Pixels < 0 && length.Percentage == 0 {
		return current
	}
	return SizeValue{Kind: SizeLength, Value: length}
}

func initialSizeValue(property string) SizeValue {
	if property == "width" || property == "height" || property == "min-width" || property == "min-height" {
		return SizeValue{Kind: SizeAuto}
	}
	if strings.HasPrefix(property, "max-") {
		return SizeValue{Kind: SizeNone}
	}
	return SizeValue{Kind: SizeLength}
}

func resolveLineHeight(value string, parent, fontSize float32, context LengthContext) (float32, bool) {
	switch parseGlobalKeyword(value) {
	case globalInherit, globalUnset:
		return parent, true
	case globalInitial:
		return 0, true
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "normal" {
		return 0, true
	}
	if multiplier, err := strconv.ParseFloat(value, 32); err == nil {
		return float32(multiplier) * fontSize, multiplier >= 0
	}
	context.FontSize, context.PercentageBase = fontSize, fontSize
	length, ok := ResolveLength(value, context)
	resolved := length.Resolve(fontSize)
	return resolved, ok && resolved >= 0
}

func resolveWhiteSpace(value string, parent WhiteSpace) (WhiteSpace, bool) {
	switch parseGlobalKeyword(value) {
	case globalInherit, globalUnset:
		return parent, true
	case globalInitial:
		return WhiteSpaceNormal, true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "normal":
		return WhiteSpaceNormal, true
	case "nowrap":
		return WhiteSpaceNowrap, true
	case "pre":
		return WhiteSpacePre, true
	case "pre-wrap":
		return WhiteSpacePreWrap, true
	case "pre-line":
		return WhiteSpacePreLine, true
	default:
		return 0, false
	}
}

func resolveOverflow(value string, parent Overflow) (Overflow, bool) {
	switch parseGlobalKeyword(value) {
	case globalInherit:
		return parent, true
	case globalInitial, globalUnset:
		return OverflowVisible, true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "visible":
		return OverflowVisible, true
	case "hidden":
		return OverflowHidden, true
	case "auto":
		return OverflowAuto, true
	case "scroll":
		return OverflowScroll, true
	default:
		return 0, false
	}
}

func applyCustomProperties(inherited map[string]string, winners map[string]winner, stylesheet *css.Stylesheet, context LengthContext, parentValues map[string]string) map[string]string {
	result := inherited
	for property, candidate := range winners {
		if !strings.HasPrefix(property, "--") {
			continue
		}
		switch parseGlobalKeyword(candidate.value) {
		case globalInitial:
			if _, registered := registeredProperty(stylesheet, property); !registered && result != nil {
				delete(result, property)
			}
		case globalInherit:
			if parentValue, ok := parentValues[property]; ok {
				result[property] = parentValue
			}
		case globalUnset:
			if registration, registered := registeredProperty(stylesheet, property); !registered || registration.Inherits {
				if parentValue, ok := parentValues[property]; ok {
					result[property] = parentValue
				}
			}
		default:
			value := candidate.value
			if registration, ok := registeredProperty(stylesheet, property); ok {
				resolved, valid := resolveVariables(value, result)
				if !valid || !validRegisteredValue(registration.Syntax, resolved, context) {
					continue
				}
				value = resolved
			}
			if result == nil {
				result = make(map[string]string)
			}
			result[property] = value
		}
	}
	return result
}

func registeredPropertyBase(inherited map[string]string, stylesheet *css.Stylesheet, context LengthContext) map[string]string {
	result := make(map[string]string, len(inherited))
	for name, value := range inherited {
		result[name] = value
	}
	if stylesheet == nil {
		return result
	}
	for _, registration := range stylesheet.Properties {
		if !registration.Valid || !validRegisteredValue(registration.Syntax, registration.InitialValue, context) {
			continue
		}
		if registration.Inherits {
			if _, ok := result[registration.Name]; ok {
				continue
			}
		}
		result[registration.Name] = registration.InitialValue
	}
	return result
}

func registeredProperty(stylesheet *css.Stylesheet, name string) (css.PropertyRule, bool) {
	if stylesheet == nil {
		return css.PropertyRule{}, false
	}
	for index := len(stylesheet.Properties) - 1; index >= 0; index-- {
		if stylesheet.Properties[index].Name == name && stylesheet.Properties[index].Valid {
			return stylesheet.Properties[index], true
		}
	}
	return css.PropertyRule{}, false
}

func resolveVariables(value string, customProperties map[string]string) (string, bool) {
	return resolveVariablesWithStack(value, customProperties, make(map[string]bool))
}

func resolveVariablesWithStack(value string, customProperties map[string]string, resolving map[string]bool) (string, bool) {
	var result strings.Builder
	for position := 0; position < len(value); {
		functionStart := findVarFunction(value, position)
		if functionStart < 0 {
			result.WriteString(value[position:])
			break
		}
		result.WriteString(value[position:functionStart])
		open := functionStart + 3
		end, ok := cssFunctionEnd(value, open)
		if !ok {
			return "", false
		}
		name, fallback, hasFallback := splitVarArguments(value[open+1 : end])
		name = strings.TrimSpace(name)
		if !strings.HasPrefix(name, "--") || !validCustomPropertyName(name) {
			return "", false
		}
		replacement, found := customProperties[name]
		resolved := ""
		if found && !resolving[name] {
			resolving[name] = true
			resolved, found = resolveVariablesWithStack(replacement, customProperties, resolving)
			delete(resolving, name)
		} else {
			found = false
		}
		if !found {
			if !hasFallback {
				return "", false
			}
			resolved, found = resolveVariablesWithStack(fallback, customProperties, resolving)
			if !found {
				return "", false
			}
		}
		result.WriteString(resolved)
		position = end + 1
	}
	return result.String(), true
}

func findVarFunction(value string, start int) int {
	var quote byte
	escaped := false
	for position := start; position+4 <= len(value); position++ {
		character := value[position]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if strings.EqualFold(value[position:position+4], "var(") &&
			(position == 0 || !isCSSNameByte(value[position-1])) {
			return position
		}
	}
	return -1
}

func cssFunctionEnd(value string, open int) (int, bool) {
	if open >= len(value) || value[open] != '(' {
		return 0, false
	}
	depth := 1
	var quote byte
	escaped := false
	for position := open + 1; position < len(value); position++ {
		character := value[position]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return position, true
			}
		}
	}
	return 0, false
}

func splitVarArguments(value string) (string, string, bool) {
	depth := 0
	var quote byte
	escaped := false
	for position := 0; position < len(value); position++ {
		character := value[position]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				return value[:position], value[position+1:], true
			}
		}
	}
	return value, "", false
}

func validCustomPropertyName(value string) bool {
	if len(value) <= 2 {
		return false
	}
	for position := 2; position < len(value); position++ {
		if !isCSSNameByte(value[position]) {
			return false
		}
	}
	return true
}

func isCSSNameByte(value byte) bool {
	return value == '-' || value == '_' || value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value >= 0x80
}

func applyGeneratedContent(node *dom.Node, computed ComputedStyle, stylesheet *css.Stylesheet, state InteractionState, environment Environment, computedAncestors Map) ComputedStyle {
	if stylesheet == nil {
		return computed
	}
	for _, target := range []struct {
		kind        css.PseudoElementKind
		destination *string
	}{
		{css.PseudoElementBefore, &computed.BeforeContent},
		{css.PseudoElementAfter, &computed.AfterContent},
	} {
		layerOrders := make(map[string]int, len(stylesheet.LayerOrder))
		for index, layer := range stylesheet.LayerOrder {
			layerOrders[layer] = index
		}
		var candidates []winner
		for _, rule := range stylesheet.Rules {
			if !matchesMediaGroups(rule.Media, environment) || !matchesSupportsGroups(rule.Supports) || !matchesContainerGroups(node, rule.Containers, computedAncestors, environment) {
				continue
			}
			for _, selector := range rule.Selectors {
				if selectorPseudoElement(selector) != target.kind || !matches(node, selector, state) {
					continue
				}
				for declarationIndex, declaration := range rule.Declarations {
					if declaration.Property != "content" {
						continue
					}
					candidate := winner{
						value: declaration.Value.Raw, source: declaration.Property, important: declaration.Important,
						specificity: selector.Specificity(), order: [2]int{rule.Order, declarationIndex}, layer: rule.Layer,
					}
					if rule.Layer != "" {
						candidate.layerOrder = layerOrders[rule.Layer]
					}
					candidates = append(candidates, candidate)
				}
			}
		}
		selected, found := selectCascadeWinner(candidates)
		if found {
			resolved, valid := resolveVariables(selected.value, computed.CustomProperties)
			if !valid {
				continue
			}
			if content, valid := parseGeneratedContent(resolved); valid {
				*target.destination = content
			}
		}
	}
	return computed
}

func selectorPseudoElement(selector css.Selector) css.PseudoElementKind {
	if len(selector.Compounds) == 0 {
		return css.PseudoElementNone
	}
	return selector.Compounds[len(selector.Compounds)-1].PseudoElement
}

func parseGeneratedContent(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "none") || strings.EqualFold(value, "normal") || parseGlobalKeyword(value) != globalNone {
		return "", true
	}
	return css.DecodeString(value)
}

func expandedProperties(property string) []string {
	switch property {
	case "margin", "padding":
		return []string{property + "-top", property + "-right", property + "-bottom", property + "-left"}
	case "margin-block", "padding-block":
		prefix := strings.TrimSuffix(property, "-block")
		return []string{prefix + "-top", prefix + "-bottom"}
	case "margin-inline", "padding-inline":
		prefix := strings.TrimSuffix(property, "-inline")
		return []string{prefix + "-left", prefix + "-right"}
	case "margin-block-start", "padding-block-start":
		return []string{strings.TrimSuffix(property, "-block-start") + "-top"}
	case "margin-block-end", "padding-block-end":
		return []string{strings.TrimSuffix(property, "-block-end") + "-bottom"}
	case "margin-inline-start", "padding-inline-start":
		return []string{strings.TrimSuffix(property, "-inline-start") + "-left"}
	case "margin-inline-end", "padding-inline-end":
		return []string{strings.TrimSuffix(property, "-inline-end") + "-right"}
	case "border":
		return borderPropertyKeys("")
	case "border-block":
		return borderLogicalKeys([]string{"top", "bottom"}, "")
	case "border-inline":
		return borderLogicalKeys([]string{"left", "right"}, "")
	case "border-block-width", "border-block-style", "border-block-color":
		return borderLogicalKeys([]string{"top", "bottom"}, strings.TrimPrefix(property, "border-block-"))
	case "border-inline-width", "border-inline-style", "border-inline-color":
		return borderLogicalKeys([]string{"left", "right"}, strings.TrimPrefix(property, "border-inline-"))
	case "border-block-start", "border-block-end", "border-inline-start", "border-inline-end":
		side := map[string]string{"border-block-start": "top", "border-block-end": "bottom", "border-inline-start": "left", "border-inline-end": "right"}[property]
		return borderLogicalKeys([]string{side}, "")
	case "border-block-start-width", "border-block-start-style", "border-block-start-color",
		"border-block-end-width", "border-block-end-style", "border-block-end-color",
		"border-inline-start-width", "border-inline-start-style", "border-inline-start-color",
		"border-inline-end-width", "border-inline-end-style", "border-inline-end-color":
		parts := strings.Split(property, "-")
		axis, edge, component := parts[1], parts[2], parts[3]
		side := map[string]string{"block-start": "top", "block-end": "bottom", "inline-start": "left", "inline-end": "right"}[axis+"-"+edge]
		return borderLogicalKeys([]string{side}, component)
	case "border-width", "border-style", "border-color":
		component := strings.TrimPrefix(property, "border-")
		return []string{"border-top-" + component, "border-right-" + component, "border-bottom-" + component, "border-left-" + component}
	case "border-top", "border-right", "border-bottom", "border-left":
		return []string{property + "-width", property + "-style", property + "-color"}
	case "overflow":
		return []string{"overflow-x", "overflow-y"}
	case "border-radius":
		return []string{"border-top-left-radius", "border-top-right-radius", "border-bottom-right-radius", "border-bottom-left-radius"}
	case "border-start-start-radius":
		return []string{"border-top-left-radius"}
	case "border-start-end-radius":
		return []string{"border-top-right-radius"}
	case "border-end-start-radius":
		return []string{"border-bottom-left-radius"}
	case "border-end-end-radius":
		return []string{"border-bottom-right-radius"}
	case "flex-flow":
		return []string{"flex-direction", "flex-wrap"}
	case "flex":
		return []string{"flex-grow", "flex-shrink", "flex-basis"}
	case "gap":
		return []string{"row-gap", "column-gap"}
	case "place-content":
		return []string{"align-content", "justify-content"}
	case "place-items":
		return []string{"align-items", "justify-items"}
	case "place-self":
		return []string{"align-self", "justify-self"}
	case "inset":
		return []string{"top", "right", "bottom", "left"}
	case "inset-block":
		return []string{"top", "bottom"}
	case "inset-inline":
		return []string{"left", "right"}
	case "inset-block-start":
		return []string{"top"}
	case "inset-block-end":
		return []string{"bottom"}
	case "inset-inline-start":
		return []string{"left"}
	case "inset-inline-end":
		return []string{"right"}
	case "inline-size":
		return []string{"width"}
	case "block-size":
		return []string{"height"}
	case "min-inline-size":
		return []string{"min-width"}
	case "min-block-size":
		return []string{"min-height"}
	case "max-inline-size":
		return []string{"max-width"}
	case "max-block-size":
		return []string{"max-height"}
	case "outline":
		return []string{"outline-width", "outline-style", "outline-color"}
	case "transition":
		return []string{"transition-property", "transition-duration", "transition-timing-function", "transition-delay"}
	case "animation":
		return []string{"animation-name", "animation-duration", "animation-timing-function", "animation-delay", "animation-iteration-count", "animation-direction", "animation-fill-mode", "animation-play-state"}
	default:
		return []string{property}
	}
}

func borderLogicalKeys(sides []string, component string) []string {
	components := []string{"width", "style", "color"}
	if component != "" {
		components = []string{component}
	}
	result := make([]string, 0, len(sides)*len(components))
	for _, side := range sides {
		for _, part := range components {
			result = append(result, "border-"+side+"-"+part)
		}
	}
	return result
}

func borderPropertyKeys(_ string) []string {
	var result []string
	for _, side := range []string{"top", "right", "bottom", "left"} {
		for _, component := range []string{"width", "style", "color"} {
			result = append(result, "border-"+side+"-"+component)
		}
	}
	return result
}

func applyEdges(edges, parent Edges, prefix string, winners map[string]winner, customProperties map[string]string, context LengthContext) Edges {
	properties := []string{prefix + "-top", prefix + "-right", prefix + "-bottom", prefix + "-left"}
	values := []*float32{&edges.Top, &edges.Right, &edges.Bottom, &edges.Left}
	parentValues := []float32{parent.Top, parent.Right, parent.Bottom, parent.Left}
	for index, property := range properties {
		candidate, ok := winners[property]
		if !ok {
			continue
		}
		resolved, ok := resolveVariables(candidate.value, customProperties)
		if !ok {
			continue
		}
		var parsed float32
		var valid bool
		switch parseGlobalKeyword(resolved) {
		case globalInherit:
			parsed, valid = parentValues[index], true
		case globalInitial, globalUnset:
			parsed, valid = 0, true
		default:
			component, componentValid := logicalEdgeComponent(candidate.source, resolved, index, prefix)
			if !componentValid {
				continue
			}
			parsed, valid = parseEdgeValue(component, index, context)
			if prefix == "padding" && parsed < 0 {
				valid = false
			}
		}
		if valid {
			*values[index] = parsed
		}
	}
	return edges
}

func applyBorders(border, parent Borders, winners map[string]winner, customProperties map[string]string, context LengthContext, currentColor uint32) Borders {
	sides := []*BorderSide{&border.Top, &border.Right, &border.Bottom, &border.Left}
	parentSides := []BorderSide{parent.Top, parent.Right, parent.Bottom, parent.Left}
	sideNames := []string{"top", "right", "bottom", "left"}
	for sideIndex, sideName := range sideNames {
		for _, component := range []string{"width", "style", "color"} {
			candidate, ok := winners["border-"+sideName+"-"+component]
			if !ok {
				continue
			}
			resolved, ok := resolveVariables(candidate.value, customProperties)
			if !ok {
				continue
			}
			switch parseGlobalKeyword(resolved) {
			case globalInherit:
				setBorderComponent(sides[sideIndex], component, parentSides[sideIndex])
				continue
			case globalInitial, globalUnset:
				setBorderComponent(sides[sideIndex], component, BorderSide{Color: currentColor})
				continue
			}
			value, ok := borderComponentValue(candidate.source, component, sideIndex, resolved)
			if !ok {
				continue
			}
			switch component {
			case "width":
				width, valid := parseBorderWidth(value, context)
				if valid {
					sides[sideIndex].Width = width
				}
			case "style":
				style, valid := parseBorderStyle(value)
				if valid {
					sides[sideIndex].Style = style
				}
			case "color":
				color, valid := parseColor(value, currentColor)
				if valid {
					sides[sideIndex].Color = color
				}
			}
		}
		if sides[sideIndex].Style == BorderNone {
			sides[sideIndex].Width = 0
		}
	}
	return border
}

func setBorderComponent(target *BorderSide, component string, source BorderSide) {
	switch component {
	case "width":
		target.Width = source.Width
	case "style":
		target.Style = source.Style
	case "color":
		target.Color = source.Color
	}
}

func borderComponentValue(source, component string, side int, value string) (string, bool) {
	if source == "border" || source == "border-top" || source == "border-right" || source == "border-bottom" || source == "border-left" ||
		source == "border-block" || source == "border-inline" || source == "border-block-start" || source == "border-block-end" || source == "border-inline-start" || source == "border-inline-end" {
		parts, ok := splitCSSSpaceSeparated(value)
		if !ok {
			return "", false
		}
		for _, part := range parts {
			switch component {
			case "width":
				if _, valid := parseBorderWidth(part, LengthContext{FontSize: 16, RootFontSize: 16}); valid {
					return part, true
				}
			case "style":
				if _, valid := parseBorderStyle(part); valid {
					return part, true
				}
			case "color":
				if _, valid := parseColor(part, 0); valid {
					return part, true
				}
			}
		}
		switch component {
		case "width":
			return "medium", true
		case "style":
			return "none", true
		case "color":
			return "currentcolor", true
		}
	}
	if source == "border-width" || source == "border-style" || source == "border-color" {
		parts, ok := splitCSSSpaceSeparated(value)
		if !ok || len(parts) < 1 || len(parts) > 4 {
			return "", false
		}
		return edgePart(parts, side), true
	}
	if strings.HasPrefix(source, "border-block-") || strings.HasPrefix(source, "border-inline-") {
		parts, ok := splitCSSSpaceSeparated(value)
		if !ok || len(parts) < 1 || len(parts) > 2 {
			return "", false
		}
		if source == "border-block-width" || source == "border-block-style" || source == "border-block-color" {
			if side == 2 && len(parts) == 2 {
				return parts[1], true
			}
			return parts[0], true
		}
		if source == "border-inline-width" || source == "border-inline-style" || source == "border-inline-color" {
			if side == 1 && len(parts) == 2 {
				return parts[1], true
			}
			return parts[0], true
		}
	}
	return value, true
}

func logicalEdgeComponent(source, value string, side int, prefix string) (string, bool) {
	if source == prefix {
		return value, true
	}
	parts, ok := splitCSSSpaceSeparated(value)
	if !ok || len(parts) == 0 {
		return "", false
	}
	switch source {
	case prefix + "-block":
		if len(parts) > 2 || side != 0 && side != 2 {
			return "", false
		}
		if side == 2 && len(parts) == 2 {
			return parts[1], true
		}
		return parts[0], true
	case prefix + "-inline":
		if len(parts) > 2 || side != 1 && side != 3 {
			return "", false
		}
		if side == 1 && len(parts) == 2 {
			return parts[1], true
		}
		return parts[0], true
	default:
		return value, true
	}
}

func edgePart(parts []string, side int) string {
	switch len(parts) {
	case 1:
		return parts[0]
	case 2:
		return parts[side%2]
	case 3:
		return []string{parts[0], parts[1], parts[2], parts[1]}[side]
	default:
		return parts[side]
	}
}

func parseBorderWidth(value string, context LengthContext) (float32, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "thin":
		return 1, true
	case "medium":
		return 3, true
	case "thick":
		return 5, true
	}
	length, ok := ResolveLength(value, context)
	return length.Pixels, ok && length.Percentage == 0 && length.Pixels >= 0
}

func parseBorderStyle(value string) (BorderStyle, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none", "hidden":
		return BorderNone, true
	case "solid":
		return BorderSolid, true
	case "dotted":
		return BorderDotted, true
	case "dashed":
		return BorderDashed, true
	case "double":
		return BorderDouble, true
	default:
		return 0, false
	}
}

func parseEdgeValue(value string, side int, context LengthContext) (float32, bool) {
	parts, ok := splitCSSSpaceSeparated(value)
	if !ok {
		return 0, false
	}
	if len(parts) < 1 || len(parts) > 4 {
		return 0, false
	}
	resolved := [4]string{}
	switch len(parts) {
	case 1:
		resolved = [4]string{parts[0], parts[0], parts[0], parts[0]}
	case 2:
		resolved = [4]string{parts[0], parts[1], parts[0], parts[1]}
	case 3:
		resolved = [4]string{parts[0], parts[1], parts[2], parts[1]}
	case 4:
		resolved = [4]string{parts[0], parts[1], parts[2], parts[3]}
	}
	length, valid := ResolveLength(resolved[side], context)
	return length.Resolve(context.PercentageBase), valid
}

func splitCSSSpaceSeparated(value string) ([]string, bool) {
	var parts []string
	start, depth := -1, 0
	var quote byte
	escaped := false
	for position := 0; position < len(value); position++ {
		character := value[position]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"':
			if start < 0 {
				start = position
			}
			quote = character
		case '(':
			if start < 0 {
				start = position
			}
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil, false
			}
		default:
			if isCSSWhitespaceByte(character) && depth == 0 {
				if start >= 0 {
					parts = append(parts, value[start:position])
					start = -1
				}
			} else if start < 0 {
				start = position
			}
		}
	}
	if quote != 0 || depth != 0 {
		return nil, false
	}
	if start >= 0 {
		parts = append(parts, value[start:])
	}
	return parts, true
}

func parseDisplay(value string) (Display, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "inline":
		return DisplayInline, true
	case "block":
		return DisplayBlock, true
	case "inline-block":
		return DisplayInlineBlock, true
	case "flex":
		return DisplayFlex, true
	case "inline-flex":
		return DisplayInlineFlex, true
	case "grid":
		return DisplayGrid, true
	case "inline-grid":
		return DisplayInlineGrid, true
	case "none":
		return DisplayNone, true
	default:
		return DisplayInline, false
	}
}

func matches(node *dom.Node, selector css.Selector, state InteractionState) bool {
	if node == nil || node.Type != dom.NodeElement {
		return false
	}
	if len(selector.Compounds) != 0 {
		return matchesSelectorAt(node, selector, len(selector.Compounds)-1, state)
	}
	if selector.Tag != "" && node.TagName != selector.Tag {
		return false
	}
	if selector.Hover && !state.Hovered[node.ID] {
		return false
	}
	if selector.ID != "" {
		id, _ := node.Attribute("id")
		if id != selector.ID {
			return false
		}
	}
	if selector.Class != "" {
		classes, _ := node.Attribute("class")
		for _, class := range strings.Fields(classes) {
			if class == selector.Class {
				return true
			}
		}
		return false
	}
	return true
}

// MatchesSelector exposes the style engine's selector semantics to DOM APIs.
func MatchesSelector(node *dom.Node, selector css.Selector, state InteractionState) bool {
	return matches(node, selector, state)
}

func matchesSelectorAt(node *dom.Node, selector css.Selector, index int, state InteractionState) bool {
	if !matchesCompound(node, selector.Compounds[index], state) {
		return false
	}
	if index == 0 {
		return true
	}
	switch selector.Combinators[index-1] {
	case css.CombinatorDescendant:
		for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
			if ancestor.Type == dom.NodeDocumentFragment {
				return false
			}
			if ancestor.Type == dom.NodeElement && matchesSelectorAt(ancestor, selector, index-1, state) {
				return true
			}
		}
		return false
	case css.CombinatorChild:
		return node.Parent != nil && node.Parent.Type == dom.NodeElement &&
			matchesSelectorAt(node.Parent, selector, index-1, state)
	case css.CombinatorAdjacentSibling:
		previous := previousElementSibling(node)
		return previous != nil && matchesSelectorAt(previous, selector, index-1, state)
	case css.CombinatorGeneralSibling:
		for previous := previousElementSibling(node); previous != nil; previous = previousElementSibling(previous) {
			if matchesSelectorAt(previous, selector, index-1, state) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func previousElementSibling(node *dom.Node) *dom.Node {
	if node == nil || node.Parent == nil {
		return nil
	}
	var previous *dom.Node
	for _, sibling := range node.Parent.Children {
		if sibling == node {
			return previous
		}
		if sibling.Type == dom.NodeElement {
			previous = sibling
		}
	}
	return nil
}

func matchesCompound(node *dom.Node, compound css.CompoundSelector, state InteractionState) bool {
	if compound.Type != "" && node.TagName != compound.Type {
		return false
	}
	if compound.Hover && !state.Hovered[node.ID] {
		return false
	}
	id, _ := node.Attribute("id")
	for _, expected := range compound.IDs {
		if id != expected {
			return false
		}
	}
	classes, _ := node.Attribute("class")
	available := make(map[string]bool)
	for _, class := range strings.Fields(classes) {
		available[class] = true
	}
	for _, expected := range compound.Classes {
		if !available[expected] {
			return false
		}
	}
	for _, attribute := range compound.Attributes {
		if !matchesAttribute(node, attribute) {
			return false
		}
	}
	for _, pseudo := range compound.Pseudos {
		if !matchesPseudoClass(node, pseudo, state) {
			return false
		}
	}
	return true
}

func matchesPseudoClass(node *dom.Node, pseudo css.PseudoClass, state InteractionState) bool {
	switch pseudo.Kind {
	case css.PseudoRoot:
		return node.Parent != nil && node.Parent.Type == dom.NodeDocument
	case css.PseudoEmpty:
		for _, child := range node.Children {
			if child.Type == dom.NodeElement || child.Type == dom.NodeText && child.Text != "" {
				return false
			}
		}
		return true
	case css.PseudoFirstChild:
		return elementPosition(node, false, false) == 1
	case css.PseudoLastChild:
		return elementPosition(node, true, false) == 1
	case css.PseudoOnlyChild:
		return elementPosition(node, false, false) == 1 && elementPosition(node, true, false) == 1
	case css.PseudoFirstOfType:
		return elementPosition(node, false, true) == 1
	case css.PseudoLastOfType:
		return elementPosition(node, true, true) == 1
	case css.PseudoOnlyOfType:
		return elementPosition(node, false, true) == 1 && elementPosition(node, true, true) == 1
	case css.PseudoNthChild:
		return matchesNth(elementPosition(node, false, false), pseudo.A, pseudo.B)
	case css.PseudoNthLastChild:
		return matchesNth(elementPosition(node, true, false), pseudo.A, pseudo.B)
	case css.PseudoNthOfType:
		return matchesNth(elementPosition(node, false, true), pseudo.A, pseudo.B)
	case css.PseudoNthLastOfType:
		return matchesNth(elementPosition(node, true, true), pseudo.A, pseudo.B)
	case css.PseudoNot:
		if len(pseudo.Selectors) == 0 && pseudo.Negation != nil {
			return !matchesCompound(node, *pseudo.Negation, state)
		}
		for _, selector := range pseudo.Selectors {
			if matches(node, selector, state) {
				return false
			}
		}
		return len(pseudo.Selectors) != 0
	case css.PseudoIs, css.PseudoWhere:
		for _, selector := range pseudo.Selectors {
			if matches(node, selector, state) {
				return true
			}
		}
		return false
	case css.PseudoHas:
		return matchesHas(node, pseudo.Selectors, state)
	case css.PseudoScope, css.PseudoRelativeScope:
		if state.Scope != 0 {
			return state.Scope == node.ID
		}
		return node.Parent != nil && node.Parent.Type == dom.NodeDocument
	case css.PseudoLink:
		_, hasReference := node.Attribute("href")
		return (node.TagName == "a" || node.TagName == "area") && hasReference
	case css.PseudoFocus:
		return state.Focused != 0 && state.Focused == node.ID
	case css.PseudoEnabled:
		return isFormControl(node) && !isDisabled(node)
	case css.PseudoDisabled:
		return isFormControl(node) && isDisabled(node)
	case css.PseudoChecked:
		if node.TagName == "option" {
			_, selected := node.Attribute("selected")
			return selected
		}
		inputType, _ := node.Attribute("type")
		checked := forms.CurrentChecked(node)
		return node.TagName == "input" && checked && (strings.EqualFold(inputType, "checkbox") || strings.EqualFold(inputType, "radio"))
	case css.PseudoValid:
		return forms.WillValidate(node) && forms.ValidateControl(state.Document, node).Valid()
	case css.PseudoInvalid:
		return forms.WillValidate(node) && !forms.ValidateControl(state.Document, node).Valid()
	case css.PseudoDefined:
		return !strings.Contains(node.TagName, "-")
	case css.PseudoPlaceholderShown:
		_, placeholder := node.Attribute("placeholder")
		return placeholder && (node.TagName == "textarea" || node.TagName == "input") && forms.CurrentValue(node) == ""
	case css.PseudoReadOnly:
		return !isReadWrite(node)
	case css.PseudoReadWrite:
		return isReadWrite(node)
	case css.PseudoRequired:
		_, required := node.Attribute("required")
		return supportsRequired(node) && required
	case css.PseudoOptional:
		_, required := node.Attribute("required")
		return supportsRequired(node) && !required
	case css.PseudoFocusVisible:
		return state.Focused == node.ID && (state.FocusVisible || forms.IsEditableTextControl(node))
	case css.PseudoFocusWithin:
		return focusWithin(node, state)
	default:
		return false
	}
}

func isReadWrite(node *dom.Node) bool {
	if node == nil || forms.Disabled(node) {
		return false
	}
	if forms.IsEditableTextControl(node) {
		return !forms.ReadOnly(node)
	}
	editable, exists := node.Attribute("contenteditable")
	return exists && (strings.EqualFold(strings.TrimSpace(editable), "true") || editable == "")
}

func supportsRequired(node *dom.Node) bool {
	if node == nil {
		return false
	}
	return node.TagName == "input" || node.TagName == "select" || node.TagName == "textarea"
}

func focusWithin(node *dom.Node, state InteractionState) bool {
	if node == nil || state.Focused == 0 || state.Document == nil {
		return false
	}
	focused, ok := state.Document.NodeByID(state.Focused)
	if !ok {
		return false
	}
	for current := focused; current != nil; current = current.Parent {
		if current == node {
			return true
		}
	}
	return false
}

func matchesHas(anchor *dom.Node, selectors []css.Selector, state InteractionState) bool {
	if anchor == nil || len(selectors) == 0 {
		return false
	}
	root := anchor
	for root.Parent != nil && root.Parent.Type != dom.NodeDocumentFragment {
		root = root.Parent
	}
	state.Scope = anchor.ID
	visited := 0
	var walk func(*dom.Node) bool
	walk = func(candidate *dom.Node) bool {
		if candidate == nil || visited >= 50_000 {
			return false
		}
		visited++
		if candidate.Type == dom.NodeElement {
			for _, selector := range selectors {
				if matches(candidate, selector, state) {
					return true
				}
			}
		}
		if candidate.Type == dom.NodeElement && candidate.TagName == "template" {
			return false
		}
		for _, child := range candidate.Children {
			if walk(child) {
				return true
			}
		}
		return false
	}
	return walk(root)
}

func isFormControl(node *dom.Node) bool {
	if node == nil {
		return false
	}
	switch node.TagName {
	case "button", "input", "select", "textarea", "option", "optgroup", "fieldset":
		return true
	default:
		return false
	}
}

func isDisabled(node *dom.Node) bool {
	_, disabled := node.Attribute("disabled")
	if disabled {
		return true
	}
	if node.TagName == "option" && node.Parent != nil && node.Parent.TagName == "optgroup" {
		_, disabled = node.Parent.Attribute("disabled")
	}
	return disabled
}

func elementPosition(node *dom.Node, fromEnd, sameType bool) int {
	if node == nil || node.Parent == nil {
		return 0
	}
	position := 0
	if fromEnd {
		for index := len(node.Parent.Children) - 1; index >= 0; index-- {
			sibling := node.Parent.Children[index]
			if sibling.Type != dom.NodeElement || sameType && sibling.TagName != node.TagName {
				continue
			}
			position++
			if sibling == node {
				return position
			}
		}
		return 0
	}
	for _, sibling := range node.Parent.Children {
		if sibling.Type != dom.NodeElement || sameType && sibling.TagName != node.TagName {
			continue
		}
		position++
		if sibling == node {
			return position
		}
	}
	return 0
}

func matchesNth(position, a, b int) bool {
	if position <= 0 {
		return false
	}
	if a == 0 {
		return position == b
	}
	difference := position - b
	return difference%a == 0 && difference/a >= 0
}

func matchesAttribute(node *dom.Node, selector css.AttributeSelector) bool {
	value, present := node.Attribute(selector.Name)
	if selector.Matcher == css.AttributePresent {
		return present
	}
	if !present {
		return false
	}
	switch selector.Matcher {
	case css.AttributeExact:
		return value == selector.Value
	case css.AttributeIncludes:
		for _, word := range strings.Fields(value) {
			if word == selector.Value {
				return true
			}
		}
		return false
	case css.AttributeDashMatch:
		return value == selector.Value || strings.HasPrefix(value, selector.Value+"-")
	case css.AttributePrefix:
		return selector.Value != "" && strings.HasPrefix(value, selector.Value)
	case css.AttributeSuffix:
		return selector.Value != "" && strings.HasSuffix(value, selector.Value)
	case css.AttributeSubstring:
		return selector.Value != "" && strings.Contains(value, selector.Value)
	default:
		return false
	}
}

func outranks(candidate, current winner) bool {
	if candidate.important != current.important {
		return candidate.important
	}
	if candidate.inline != current.inline {
		return candidate.inline
	}
	candidateLayered, currentLayered := candidate.layer != "", current.layer != ""
	if candidateLayered != currentLayered {
		if candidate.important {
			return candidateLayered
		}
		return !candidateLayered
	}
	if candidateLayered && candidate.layerOrder != current.layerOrder {
		if candidate.important {
			return candidate.layerOrder < current.layerOrder
		}
		return candidate.layerOrder > current.layerOrder
	}
	for index := range candidate.specificity {
		if candidate.specificity[index] != current.specificity[index] {
			return candidate.specificity[index] > current.specificity[index]
		}
	}
	if candidate.order[0] != current.order[0] {
		return candidate.order[0] > current.order[0]
	}
	return candidate.order[1] >= current.order[1]
}

func selectCascadeWinner(candidates []winner) (winner, bool) {
	remaining := append([]winner(nil), candidates...)
	for len(remaining) != 0 {
		selectedIndex := 0
		for index := 1; index < len(remaining); index++ {
			if outranks(remaining[index], remaining[selectedIndex]) {
				selectedIndex = index
			}
		}
		selected := remaining[selectedIndex]
		if !strings.EqualFold(strings.TrimSpace(selected.value), "revert-layer") {
			return selected, true
		}
		filtered := remaining[:0]
		for _, candidate := range remaining {
			if sameCascadeLayer(candidate, selected) {
				continue
			}
			filtered = append(filtered, candidate)
		}
		remaining = filtered
	}
	return winner{}, false
}

func sameCascadeLayer(left, right winner) bool {
	if left.inline || right.inline {
		return left.inline == right.inline
	}
	return left.layer == right.layer
}

func parseFontWeight(value string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "normal":
		return 400, true
	case "bold":
		return 700, true
	}
	weight, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || weight < 1 || weight > 1000 {
		return 0, false
	}
	return weight, true
}
