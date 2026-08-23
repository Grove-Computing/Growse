package fetch

import (
	"errors"
	"net/http"
	"strings"
)

const (
	maxHeaderFields    = 100
	maxHeaderNameSize  = 256
	maxHeaderValueSize = 8 * 1024
	maxHeaderBytes     = 64 * 1024
)

var (
	ErrInvalidHeader   = errors.New("invalid Fetch header")
	ErrForbiddenHeader = errors.New("forbidden Fetch request header")
	ErrHeadersTooLarge = errors.New("Fetch headers exceed size limit")
)

// HeaderEntry is one ordered HTTP field name/value pair.
type HeaderEntry struct{ Name, Value string }

// Headers stores ordered HTTP fields while preserving repeated values.
type Headers struct{ entries []HeaderEntry }

// NewHeaders creates an empty request header collection.
func NewHeaders() *Headers { return &Headers{} }

// Append adds one validated request header field.
func (headers *Headers) Append(name, value string) error {
	if headers == nil {
		return ErrInvalidHeader
	}
	updated := append(append([]HeaderEntry(nil), headers.entries...), HeaderEntry{Name: name, Value: value})
	if err := validateHeaderEntries(updated); err != nil {
		return err
	}
	headers.entries = updated
	return nil
}

// Set replaces all values for name while retaining the first matching position.
func (headers *Headers) Set(name, value string) error {
	if headers == nil {
		return ErrInvalidHeader
	}
	updated := make([]HeaderEntry, 0, len(headers.entries)+1)
	replaced := false
	for _, entry := range headers.entries {
		if !strings.EqualFold(entry.Name, name) {
			updated = append(updated, entry)
			continue
		}
		if !replaced {
			updated = append(updated, HeaderEntry{Name: name, Value: value})
			replaced = true
		}
	}
	if !replaced {
		updated = append(updated, HeaderEntry{Name: name, Value: value})
	}
	if err := validateHeaderEntries(updated); err != nil {
		return err
	}
	headers.entries = updated
	return nil
}

// Get returns the first value for name.
func (headers *Headers) Get(name string) (string, bool) {
	if headers == nil {
		return "", false
	}
	for _, entry := range headers.entries {
		if strings.EqualFold(entry.Name, name) {
			return entry.Value, true
		}
	}
	return "", false
}

// Values returns all values for name in insertion order.
func (headers *Headers) Values(name string) []string {
	if headers == nil {
		return nil
	}
	values := make([]string, 0)
	for _, entry := range headers.entries {
		if strings.EqualFold(entry.Name, name) {
			values = append(values, entry.Value)
		}
	}
	return values
}

// Has reports whether name has at least one value.
func (headers *Headers) Has(name string) bool { _, ok := headers.Get(name); return ok }

// Delete removes all values for name.
func (headers *Headers) Delete(name string) {
	if headers == nil {
		return
	}
	updated := headers.entries[:0]
	for _, entry := range headers.entries {
		if !strings.EqualFold(entry.Name, name) {
			updated = append(updated, entry)
		}
	}
	headers.entries = updated
}

// Entries returns a defensive ordered copy.
func (headers *Headers) Entries() []HeaderEntry {
	if headers == nil {
		return nil
	}
	return append([]HeaderEntry(nil), headers.entries...)
}

// Clone returns an independent header collection.
func (headers *Headers) Clone() *Headers { return &Headers{entries: headers.Entries()} }

func (headers *Headers) httpHeader() (http.Header, error) {
	if headers == nil {
		return make(http.Header), nil
	}
	if err := validateHeaderEntries(headers.entries); err != nil {
		return nil, err
	}
	result := make(http.Header)
	for _, entry := range headers.entries {
		result.Add(entry.Name, entry.Value)
	}
	return result, nil
}

func validateHeaderEntries(entries []HeaderEntry) error {
	if len(entries) > maxHeaderFields {
		return ErrHeadersTooLarge
	}
	total := 0
	for _, entry := range entries {
		if len(entry.Name) == 0 || len(entry.Name) > maxHeaderNameSize || !validToken(entry.Name) || strings.ContainsAny(entry.Value, "\r\n\x00") || len(entry.Value) > maxHeaderValueSize {
			return ErrInvalidHeader
		}
		if forbiddenHeader(entry.Name) {
			return ErrForbiddenHeader
		}
		total += len(entry.Name) + len(entry.Value)
		if total > maxHeaderBytes {
			return ErrHeadersTooLarge
		}
	}
	return nil
}

func legacyHeaders(header Header) (*Headers, error) {
	result := NewHeaders()
	for name, values := range header {
		for _, value := range values {
			if err := result.Append(name, value); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}
