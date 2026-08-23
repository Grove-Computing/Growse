// Package form provides form data values exposed to WebGo programs.
package form

import (
	"errors"
	stdurl "net/url"
	"strings"
	"unicode/utf8"
)

const (
	ContentTypeURLEncoded = "application/x-www-form-urlencoded;charset=UTF-8"
	maxFields             = 100
	maxNameSize           = 256
	maxValueSize          = 64 * 1024
	maxBodySize           = 1024 * 1024
)

var (
	ErrInvalidField = errors.New("invalid FormData field")
	ErrTooLarge     = errors.New("FormData exceeds size limit")
)

// Entry is one ordered FormData field name/value pair.
type Entry struct{ Name, Value string }

// FormData stores ordered text form fields. File and multipart entries are not supported.
type FormData struct{ entries []Entry }

// New creates an empty FormData value.
func New() *FormData { return &FormData{} }

// Append adds a text field without removing existing values for name.
func (data *FormData) Append(name, value string) error {
	if data == nil {
		return ErrInvalidField
	}
	updated := append(append([]Entry(nil), data.entries...), Entry{Name: name, Value: value})
	if err := validate(updated); err != nil {
		return err
	}
	data.entries = updated
	return nil
}

// Set replaces all values for name while retaining the first matching position.
func (data *FormData) Set(name, value string) error {
	if data == nil {
		return ErrInvalidField
	}
	updated := make([]Entry, 0, len(data.entries)+1)
	replaced := false
	for _, entry := range data.entries {
		if entry.Name != name {
			updated = append(updated, entry)
			continue
		}
		if !replaced {
			updated = append(updated, Entry{Name: name, Value: value})
			replaced = true
		}
	}
	if !replaced {
		updated = append(updated, Entry{Name: name, Value: value})
	}
	if err := validate(updated); err != nil {
		return err
	}
	data.entries = updated
	return nil
}

// Get returns the first value for name.
func (data *FormData) Get(name string) (string, bool) {
	if data == nil {
		return "", false
	}
	for _, entry := range data.entries {
		if entry.Name == name {
			return entry.Value, true
		}
	}
	return "", false
}

// GetAll returns every value for name in insertion order.
func (data *FormData) GetAll(name string) []string {
	if data == nil {
		return nil
	}
	values := make([]string, 0)
	for _, entry := range data.entries {
		if entry.Name == name {
			values = append(values, entry.Value)
		}
	}
	return values
}

// Has reports whether name has at least one value.
func (data *FormData) Has(name string) bool { _, ok := data.Get(name); return ok }

// Delete removes all values for name.
func (data *FormData) Delete(name string) {
	if data == nil {
		return
	}
	updated := data.entries[:0]
	for _, entry := range data.entries {
		if entry.Name != name {
			updated = append(updated, entry)
		}
	}
	data.entries = updated
}

// Entries returns a defensive ordered copy.
func (data *FormData) Entries() []Entry {
	if data == nil {
		return nil
	}
	return append([]Entry(nil), data.entries...)
}

// Encode serializes fields as an application/x-www-form-urlencoded body.
func (data *FormData) Encode() (string, error) {
	if data == nil {
		return "", ErrInvalidField
	}
	if err := validate(data.entries); err != nil {
		return "", err
	}
	parts := make([]string, 0, len(data.entries))
	for _, entry := range data.entries {
		parts = append(parts, stdurl.QueryEscape(entry.Name)+"="+stdurl.QueryEscape(entry.Value))
	}
	encoded := strings.Join(parts, "&")
	if len(encoded) > maxBodySize {
		return "", ErrTooLarge
	}
	return encoded, nil
}

func validate(entries []Entry) error {
	if len(entries) > maxFields {
		return ErrTooLarge
	}
	for _, entry := range entries {
		if entry.Name == "" || len(entry.Name) > maxNameSize || len(entry.Value) > maxValueSize || !utf8.ValidString(entry.Name) || !utf8.ValidString(entry.Value) || strings.ContainsAny(entry.Name, "\r\n\x00") {
			return ErrInvalidField
		}
	}
	return nil
}
