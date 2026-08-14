package forms

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestValidateControlRequiredLengthAndNumberConstraints(t *testing.T) {
	document := dom.NewDocument()
	text := document.CreateElement("input", map[string]string{"required": "", "minlength": "3", "maxlength": "5"})
	number := document.CreateElement("input", map[string]string{"type": "number", "min": "2", "max": "10", "step": "2"})
	checkbox := document.CreateElement("input", map[string]string{"type": "checkbox", "required": ""})
	for _, node := range []*dom.Node{text, number, checkbox} {
		appendNode(t, document, document.Root, node)
	}

	if validity := ValidateControl(document, text); !validity.ValueMissing || validity.Valid() {
		t.Fatalf("required validity = %#v", validity)
	}
	SetCurrentValue(text, "ab")
	if validity := ValidateControl(document, text); !validity.TooShort || validity.Valid() {
		t.Fatalf("minlength validity = %#v", validity)
	}
	SetCurrentValue(text, "abcdef")
	if validity := ValidateControl(document, text); !validity.TooLong || validity.Valid() {
		t.Fatalf("maxlength validity = %#v", validity)
	}
	SetCurrentValue(number, "1")
	if validity := ValidateControl(document, number); !validity.RangeUnderflow {
		t.Fatalf("min validity = %#v", validity)
	}
	SetCurrentValue(number, "12")
	if validity := ValidateControl(document, number); !validity.RangeOverflow {
		t.Fatalf("max validity = %#v", validity)
	}
	SetCurrentValue(number, "5")
	if validity := ValidateControl(document, number); !validity.StepMismatch {
		t.Fatalf("step validity = %#v", validity)
	}
	if validity := ValidateControl(document, checkbox); !validity.ValueMissing {
		t.Fatalf("checkbox required validity = %#v", validity)
	}
}

func TestValidateControlAcceptsValidUnicodeLengthAndNumber(t *testing.T) {
	document := dom.NewDocument()
	text := document.CreateElement("input", map[string]string{"required": "", "minlength": "2", "maxlength": "3"})
	number := document.CreateElement("input", map[string]string{"type": "number", "min": "2", "max": "10", "step": "2"})
	appendNode(t, document, document.Root, text)
	appendNode(t, document, document.Root, number)
	SetCurrentValue(text, "日本")
	SetCurrentValue(number, "8")
	if validity := ValidateControl(document, text); !validity.Valid() {
		t.Fatalf("text validity = %#v", validity)
	}
	if validity := ValidateControl(document, number); !validity.Valid() {
		t.Fatalf("number validity = %#v", validity)
	}
}

func TestValidateControlEmailURLAndPatternMismatch(t *testing.T) {
	document := dom.NewDocument()
	email := document.CreateElement("input", map[string]string{"type": "email"})
	urlInput := document.CreateElement("input", map[string]string{"type": "url"})
	pattern := document.CreateElement("input", map[string]string{"pattern": `[A-Z]{2}-[0-9]{3}`})
	for _, node := range []*dom.Node{email, urlInput, pattern} {
		appendNode(t, document, document.Root, node)
	}

	SetCurrentValue(email, "not-an-email")
	SetCurrentValue(urlInput, "/relative")
	SetCurrentValue(pattern, "ab-123")
	if validity := ValidateControl(document, email); !validity.TypeMismatch || validity.Valid() {
		t.Fatalf("email validity = %#v", validity)
	}
	if validity := ValidateControl(document, urlInput); !validity.TypeMismatch || validity.Valid() {
		t.Fatalf("URL validity = %#v", validity)
	}
	if validity := ValidateControl(document, pattern); !validity.PatternMismatch || validity.Valid() {
		t.Fatalf("pattern validity = %#v", validity)
	}

	SetCurrentValue(email, "user@example.com")
	SetCurrentValue(urlInput, "https://example.com/path?q=1")
	SetCurrentValue(pattern, "AB-123")
	for _, node := range []*dom.Node{email, urlInput, pattern} {
		if validity := ValidateControl(document, node); !validity.Valid() {
			t.Fatalf("valid control %#v = %#v", node.Attributes, validity)
		}
	}
}

func TestValidateControlIgnoresInvalidPattern(t *testing.T) {
	document := dom.NewDocument()
	input := document.CreateElement("input", map[string]string{"pattern": "["})
	appendNode(t, document, document.Root, input)
	SetCurrentValue(input, "anything")
	if validity := ValidateControl(document, input); !validity.Valid() {
		t.Fatalf("invalid pattern constrained input = %#v", validity)
	}
}
