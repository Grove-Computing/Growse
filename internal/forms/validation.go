package forms

import (
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Grove-Computing/Growse/internal/dom"
)

// Validity is the supported subset of HTML constraint validation state.
type Validity struct {
	ValueMissing    bool
	TooShort        bool
	TooLong         bool
	RangeUnderflow  bool
	RangeOverflow   bool
	StepMismatch    bool
	TypeMismatch    bool
	PatternMismatch bool
}

// Valid reports whether none of the supported constraints failed.
func (validity Validity) Valid() bool {
	return !validity.ValueMissing && !validity.TooShort && !validity.TooLong &&
		!validity.RangeUnderflow && !validity.RangeOverflow && !validity.StepMismatch &&
		!validity.TypeMismatch && !validity.PatternMismatch
}

// ValidateControl evaluates supported constraints against browser-owned live state.
func ValidateControl(document *dom.Document, node *dom.Node) Validity {
	if !WillValidate(node) {
		return Validity{}
	}
	value := CurrentValue(node)
	validity := Validity{}
	if _, required := node.Attribute("required"); required {
		validity.ValueMissing = requiredValueMissing(document, node, value)
	}
	if IsEditableTextControl(node) {
		length := utf8.RuneCountInString(value)
		if minimum, ok := integerAttribute(node, "minlength"); ok && value != "" {
			validity.TooShort = length < minimum
		}
		if maximum, ok := integerAttribute(node, "maxlength"); ok && maximum >= 0 {
			validity.TooLong = length > maximum
		}
	}
	inputType, _ := EditableTextControlType(node)
	if inputType == "number" && strings.TrimSpace(value) != "" {
		validateNumber(node, value, &validity)
	}
	if value != "" {
		switch inputType {
		case "email":
			validity.TypeMismatch = !validEmail(value)
		case "url":
			validity.TypeMismatch = !validAbsoluteURL(value)
		}
		if pattern, exists := node.Attribute("pattern"); exists {
			if expression, err := regexp.Compile("^(?:" + pattern + ")$"); err == nil {
				validity.PatternMismatch = !expression.MatchString(value)
			}
		}
	}
	return validity
}

// WillValidate reports whether a control participates in constraint validation.
func WillValidate(node *dom.Node) bool {
	if node == nil || Disabled(node) || ReadOnly(node) {
		return false
	}
	if IsEditableTextControl(node) || node.TagName == "select" {
		return true
	}
	_, checkable := CheckableState(node)
	return checkable
}

// FirstInvalidControl returns the first invalid descendant in DOM order.
func FirstInvalidControl(document *dom.Document, form *dom.Node) (*dom.Node, bool) {
	if document == nil || form == nil || form.Type != dom.NodeElement || form.TagName != "form" {
		return nil, false
	}
	var first *dom.Node
	forEachElement(form, func(node *dom.Node) {
		if first == nil && WillValidate(node) && !ValidateControl(document, node).Valid() {
			first = node
		}
	})
	return first, first != nil
}

func validEmail(value string) bool {
	if strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\t ") || strings.Count(value, "@") != 1 {
		return false
	}
	local, domain, _ := strings.Cut(value, "@")
	return local != "" && domain != "" && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".") && !strings.Contains(domain, "..")
}

func validAbsoluteURL(value string) bool {
	if strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\t ") {
		return false
	}
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.IsAbs()
}

func requiredValueMissing(document *dom.Document, node *dom.Node, value string) bool {
	if state, checkable := CheckableState(node); checkable {
		if state.Kind == "checkbox" {
			return !state.Checked
		}
		name, _ := node.Attribute("name")
		owner := FormOwner(document, node)
		missing := !state.Checked
		if document != nil {
			forEachElement(document.Root, func(candidate *dom.Node) {
				candidateState, ok := CheckableState(candidate)
				candidateName, _ := candidate.Attribute("name")
				if ok && candidateState.Kind == "radio" && candidateName == name && FormOwner(document, candidate) == owner && candidateState.Checked {
					missing = false
				}
			})
		}
		return missing
	}
	if node.TagName == "select" {
		options := SelectOptions(node)
		index := SelectedIndex(node, options)
		return index < 0 || options[index].Value == ""
	}
	return value == ""
}

func validateNumber(node *dom.Node, rawValue string, validity *Validity) {
	value, err := strconv.ParseFloat(strings.TrimSpace(rawValue), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		validity.TypeMismatch = true
		return
	}
	minimum, hasMinimum := floatAttribute(node, "min")
	maximum, hasMaximum := floatAttribute(node, "max")
	if hasMinimum {
		validity.RangeUnderflow = value < minimum
	}
	if hasMaximum {
		validity.RangeOverflow = value > maximum
	}
	step := float64(1)
	if rawStep, exists := node.Attribute("step"); exists {
		if strings.EqualFold(strings.TrimSpace(rawStep), "any") {
			return
		}
		parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(rawStep), 64)
		if parseErr == nil && parsed > 0 {
			step = parsed
		}
	}
	base := float64(0)
	if hasMinimum {
		base = minimum
	}
	remainder := math.Mod(math.Abs(value-base), step)
	validity.StepMismatch = remainder > 1e-9 && math.Abs(step-remainder) > 1e-9
}

func integerAttribute(node *dom.Node, name string) (int, bool) {
	raw, exists := node.Attribute(name)
	if !exists {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	return value, err == nil
}

func floatAttribute(node *dom.Node, name string) (float64, bool) {
	raw, exists := node.Attribute(name)
	if !exists {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return value, err == nil && !math.IsNaN(value) && !math.IsInf(value, 0)
}
