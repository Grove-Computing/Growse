// Package url provides URL value helpers exposed to WebGo programs.
package url

import (
	"errors"
	stdurl "net/url"
	"strings"
	"unicode/utf8"
)

// MaxEncodedSize bounds both accepted query text and serialized output.
const MaxEncodedSize = 8 * 1024

var (
	// ErrInvalidQuery reports malformed percent-encoding or invalid UTF-8.
	ErrInvalidQuery = errors.New("invalid URL query")
	// ErrQueryTooLarge reports a query that exceeds MaxEncodedSize.
	ErrQueryTooLarge = errors.New("URL query exceeds size limit")
)

// Entry is one ordered URLSearchParams name/value pair.
type Entry struct {
	Key   string
	Value string
}

// URLSearchParams stores ordered query name/value pairs, including duplicate
// names and empty values.
type URLSearchParams struct {
	entries []Entry
}

// New creates an empty URLSearchParams value.
func New() *URLSearchParams { return &URLSearchParams{} }

// Parse decodes an application/x-www-form-urlencoded query string.
func Parse(raw string) (*URLSearchParams, error) {
	if len(raw) > MaxEncodedSize {
		return nil, ErrQueryTooLarge
	}
	if !utf8.ValidString(raw) {
		return nil, ErrInvalidQuery
	}
	params := New()
	if raw == "" {
		return params, nil
	}
	for _, pair := range strings.Split(raw, "&") {
		key, value, hasValue := strings.Cut(pair, "=")
		decodedKey, err := stdurl.QueryUnescape(key)
		if err != nil || !utf8.ValidString(decodedKey) {
			return nil, ErrInvalidQuery
		}
		decodedValue := ""
		if hasValue {
			decodedValue, err = stdurl.QueryUnescape(value)
			if err != nil || !utf8.ValidString(decodedValue) {
				return nil, ErrInvalidQuery
			}
		}
		params.entries = append(params.entries, Entry{Key: decodedKey, Value: decodedValue})
	}
	return params, nil
}

// Append adds one value without removing existing values for key.
func (params *URLSearchParams) Append(key, value string) error {
	if params == nil || !utf8.ValidString(key) || !utf8.ValidString(value) {
		return ErrInvalidQuery
	}
	updated := append(append([]Entry(nil), params.entries...), Entry{Key: key, Value: value})
	if err := validateEncodedSize(updated); err != nil {
		return err
	}
	params.entries = updated
	return nil
}

// Set replaces all values for key while retaining the first matching position.
func (params *URLSearchParams) Set(key, value string) error {
	if params == nil || !utf8.ValidString(key) || !utf8.ValidString(value) {
		return ErrInvalidQuery
	}
	updated := make([]Entry, 0, len(params.entries)+1)
	replaced := false
	for _, entry := range params.entries {
		if entry.Key != key {
			updated = append(updated, entry)
			continue
		}
		if !replaced {
			updated = append(updated, Entry{Key: key, Value: value})
			replaced = true
		}
	}
	if !replaced {
		updated = append(updated, Entry{Key: key, Value: value})
	}
	if err := validateEncodedSize(updated); err != nil {
		return err
	}
	params.entries = updated
	return nil
}

// Get returns the first value for key.
func (params *URLSearchParams) Get(key string) (string, bool) {
	if params == nil {
		return "", false
	}
	for _, entry := range params.entries {
		if entry.Key == key {
			return entry.Value, true
		}
	}
	return "", false
}

// GetAll returns all values for key in insertion order.
func (params *URLSearchParams) GetAll(key string) []string {
	if params == nil {
		return nil
	}
	values := make([]string, 0)
	for _, entry := range params.entries {
		if entry.Key == key {
			values = append(values, entry.Value)
		}
	}
	return values
}

// Has reports whether key has at least one value.
func (params *URLSearchParams) Has(key string) bool {
	_, ok := params.Get(key)
	return ok
}

// Delete removes every value for key.
func (params *URLSearchParams) Delete(key string) {
	if params == nil {
		return
	}
	updated := params.entries[:0]
	for _, entry := range params.entries {
		if entry.Key != key {
			updated = append(updated, entry)
		}
	}
	params.entries = updated
}

// Entries returns a defensive copy in insertion order.
func (params *URLSearchParams) Entries() []Entry {
	if params == nil {
		return nil
	}
	return append([]Entry(nil), params.entries...)
}

// Encode serializes params as application/x-www-form-urlencoded text.
func (params *URLSearchParams) Encode() (string, error) {
	if params == nil {
		return "", ErrInvalidQuery
	}
	if err := validateEncodedSize(params.entries); err != nil {
		return "", err
	}
	return encode(params.entries), nil
}

func validateEncodedSize(entries []Entry) error {
	for _, entry := range entries {
		if !utf8.ValidString(entry.Key) || !utf8.ValidString(entry.Value) {
			return ErrInvalidQuery
		}
	}
	if len(encode(entries)) > MaxEncodedSize {
		return ErrQueryTooLarge
	}
	return nil
}

func encode(entries []Entry) string {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, stdurl.QueryEscape(entry.Key)+"="+stdurl.QueryEscape(entry.Value))
	}
	return strings.Join(parts, "&")
}
