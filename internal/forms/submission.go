package forms

import (
	"fmt"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
)

const URLEncoded = "application/x-www-form-urlencoded"

// SubmissionConfig is the form configuration after submitter overrides.
type SubmissionConfig struct {
	Form       *dom.Node
	Submitter  *dom.Node
	Action     string
	Method     string
	Enctype    string
	Target     string
	NoValidate bool
}

// Entry is one successful control name/value pair.
type Entry struct {
	Name  string
	Value string
}

// CollectEntries builds the successful-control entry list in document order.
func CollectEntries(document *dom.Document, form, submitter *dom.Node) []Entry {
	if document == nil || form == nil || form.TagName != "form" {
		return nil
	}
	var entries []Entry
	forEachElement(document.Root, func(node *dom.Node) {
		if FormOwner(document, node) != form || Disabled(node) {
			return
		}
		name, exists := node.Attribute("name")
		if !exists || name == "" {
			return
		}
		if entry, successful := successfulEntry(node, submitter); successful {
			entry.Name = name
			entries = append(entries, entry)
		}
	})
	return entries
}

func successfulEntry(node, submitter *dom.Node) (Entry, bool) {
	if node.TagName == "textarea" {
		return Entry{Value: normalizeCRLF(CurrentValue(node))}, true
	}
	if node.TagName == "select" {
		options := SelectOptions(node)
		index := SelectedIndex(node, options)
		if index < 0 {
			return Entry{}, false
		}
		return Entry{Value: options[index].Value}, true
	}
	if node.TagName == "button" {
		if node != submitter || !IsSubmitButton(node) {
			return Entry{}, false
		}
		value, _ := node.Attribute("value")
		return Entry{Value: value}, true
	}
	if node.TagName != "input" {
		return Entry{}, false
	}
	typeValue, _ := node.Attribute("type")
	typeValue = strings.ToLower(strings.TrimSpace(typeValue))
	switch typeValue {
	case "checkbox", "radio":
		if !CurrentChecked(node) {
			return Entry{}, false
		}
		value, exists := node.Attribute("value")
		if !exists {
			value = "on"
		}
		return Entry{Value: value}, true
	case "submit":
		if node != submitter {
			return Entry{}, false
		}
		value, _ := node.Attribute("value")
		return Entry{Value: value}, true
	case "button", "reset", "file":
		return Entry{}, false
	default:
		return Entry{Value: CurrentValue(node)}, true
	}
}

func normalizeCRLF(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.ReplaceAll(value, "\n", "\r\n")
}

// EncodeURLEncoded serializes entries as UTF-8 application/x-www-form-urlencoded.
func EncodeURLEncoded(entries []Entry) string {
	var encoded strings.Builder
	for index, entry := range entries {
		if index != 0 {
			encoded.WriteByte('&')
		}
		encoded.WriteString(encodeFormComponent(entry.Name))
		encoded.WriteByte('=')
		encoded.WriteString(encodeFormComponent(entry.Value))
	}
	return encoded.String()
}

func encodeFormComponent(value string) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	var encoded strings.Builder
	for _, character := range []byte(value) {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9',
			character == '*', character == '-', character == '.', character == '_':
			encoded.WriteByte(character)
		case character == ' ':
			encoded.WriteByte('+')
		default:
			fmt.Fprintf(&encoded, "%%%02X", character)
		}
	}
	return encoded.String()
}

// FormOwner resolves an explicit form=id association before ancestor lookup.
func FormOwner(document *dom.Document, control *dom.Node) *dom.Node {
	if document == nil || control == nil || control.Type != dom.NodeElement {
		return nil
	}
	if ownerID, explicit := control.Attribute("form"); explicit {
		owner, found := document.GetElementByID(strings.TrimSpace(ownerID))
		if found && owner.TagName == "form" {
			return owner
		}
		return nil
	}
	return nearestForm(control)
}

// ResolveSubmission applies defaults and submitter-specific overrides.
func ResolveSubmission(document *dom.Document, submitter *dom.Node) (SubmissionConfig, bool) {
	if document == nil || submitter == nil || !document.IsConnected(submitter) || !IsSubmitButton(submitter) || Disabled(submitter) {
		return SubmissionConfig{}, false
	}
	form := FormOwner(document, submitter)
	if form == nil {
		return SubmissionConfig{}, false
	}
	return ResolveFormSubmission(document, form, submitter)
}

// ResolveFormSubmission resolves a form with an optional submitter.
func ResolveFormSubmission(document *dom.Document, form, submitter *dom.Node) (SubmissionConfig, bool) {
	if document == nil || form == nil || form.TagName != "form" || !document.IsConnected(form) {
		return SubmissionConfig{}, false
	}
	if submitter != nil && (FormOwner(document, submitter) != form || !IsSubmitButton(submitter) || Disabled(submitter)) {
		return SubmissionConfig{}, false
	}
	config := SubmissionConfig{Form: form, Submitter: submitter, Method: "get", Enctype: URLEncoded, Target: "_self"}
	config.Action, _ = form.Attribute("action")
	if method, exists := form.Attribute("method"); exists {
		config.Method = normalizeMethod(method)
	}
	if enctype, exists := form.Attribute("enctype"); exists {
		config.Enctype = normalizeEnctype(enctype)
	}
	if target, exists := form.Attribute("target"); exists && strings.TrimSpace(target) != "" {
		config.Target = strings.TrimSpace(target)
	}
	_, config.NoValidate = form.Attribute("novalidate")
	if submitter != nil {
		if value, exists := submitter.Attribute("formaction"); exists {
			config.Action = value
		}
		if value, exists := submitter.Attribute("formmethod"); exists {
			config.Method = normalizeMethod(value)
		}
		if value, exists := submitter.Attribute("formenctype"); exists {
			config.Enctype = normalizeEnctype(value)
		}
		if value, exists := submitter.Attribute("formtarget"); exists && strings.TrimSpace(value) != "" {
			config.Target = strings.TrimSpace(value)
		}
		if _, exists := submitter.Attribute("formnovalidate"); exists {
			config.NoValidate = true
		}
	}
	return config, true
}

func normalizeMethod(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "post":
		return "post"
	default:
		return "get"
	}
}

func normalizeEnctype(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), URLEncoded) {
		return URLEncoded
	}
	return URLEncoded
}
