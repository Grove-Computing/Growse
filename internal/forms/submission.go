package forms

import (
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
