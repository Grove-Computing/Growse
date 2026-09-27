// Package searchprovider owns bounded, validated browser search configuration.
package searchprovider

import (
	"errors"
	"golang.org/x/net/idna"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Grove-Computing/Growse/internal/omnibox"
)

const MaxProviders = 32

var ErrInvalid = errors.New("検索providerの設定が不正です")

type Provider struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Keyword            string `json:"keyword"`
	SearchTemplate     string `json:"searchTemplate"`
	SuggestionTemplate string `json:"suggestionTemplate,omitempty"`
	Disabled           bool   `json:"disabled,omitempty"`
}

type Settings struct {
	Providers         []Provider `json:"providers"`
	DefaultID         string     `json:"defaultID"`
	RemoteSuggestions bool       `json:"remoteSuggestions"`
}

func Builtin() Provider {
	return Provider{ID: "duckduckgo", Name: "DuckDuckGo", Keyword: "ddg", SearchTemplate: "https://duckduckgo.com/?q={searchTerms}", SuggestionTemplate: "https://duckduckgo.com/ac/?q={searchTerms}&type=list"}
}
func Defaults() Settings           { return Settings{Providers: []Provider{Builtin()}, DefaultID: "duckduckgo"} }
func (s Settings) Clone() Settings { s.Providers = append([]Provider(nil), s.Providers...); return s }

func (p Provider) Validate() error {
	if p.ID == "" || len(p.ID) > 64 || strings.IndexFunc(p.ID, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') }) >= 0 || !validText(p.Name, 128) || !validText(p.Keyword, 32) || strings.IndexFunc(p.Keyword, unicode.IsSpace) >= 0 || strings.ContainsAny(p.Keyword, "@:/?#") {
		return ErrInvalid
	}
	if err := validateTemplate(p.SearchTemplate); err != nil {
		return err
	}
	if p.SuggestionTemplate != "" {
		return validateTemplate(p.SuggestionTemplate)
	}
	return nil
}
func validText(s string, limit int) bool {
	return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= limit && strings.TrimSpace(s) == s && strings.IndexFunc(s, unicode.IsControl) < 0
}
func validateTemplate(t string) error {
	if strings.Contains(t, "#") || !utf8.ValidString(t) || strings.IndexFunc(t, unicode.IsControl) >= 0 || len(t) > omnibox.MaxInputBytes || strings.Count(t, "{searchTerms}") != 1 {
		return ErrInvalid
	}
	stripped := strings.ReplaceAll(t, "{searchTerms}", "")
	if strings.ContainsAny(stripped, "{}") {
		return ErrInvalid
	}
	u, err := url.Parse(strings.ReplaceAll(t, "{searchTerms}", "growse"))
	if err != nil || validateEndpoint(u) != nil {
		return ErrInvalid
	}
	// Queries may appear in a path or query value, never in authority or parameter names.
	original, err := url.Parse(strings.ReplaceAll(t, "{searchTerms}", "GROWSETERMS"))
	if err != nil || strings.Contains(original.Host, "GROWSETERMS") {
		return ErrInvalid
	}
	for k := range original.Query() {
		if strings.Contains(k, "GROWSETERMS") {
			return ErrInvalid
		}
	}
	return nil
}
func validateEndpoint(u *url.URL) error {
	if u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || strings.IndexFunc(u.String(), unicode.IsControl) >= 0 {
		return ErrInvalid
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return ErrInvalid
		}
	}
	host := u.Hostname()
	if net.ParseIP(host) == nil {
		ascii, err := idna.Lookup.ToASCII(host)
		if err != nil {
			return ErrInvalid
		}
		for _, label := range strings.Split(strings.TrimSuffix(ascii, "."), ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return ErrInvalid
			}
			for _, r := range label {
				if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
					return ErrInvalid
				}
			}
		}
		if len(ascii) > 253 || strings.Contains(u.Host, "[") {
			return ErrInvalid
		}
	}
	if strings.HasSuffix(u.Host, ":") || strings.ContainsAny(u.Host, "{}%") {
		return ErrInvalid
	}
	if omnibox.Classify(u.String()).Kind != omnibox.URL {
		return ErrInvalid
	}
	return nil
}
func expand(t, q string) (string, error) {
	if validateTemplate(t) != nil || !utf8.ValidString(q) || q == "" || len(q) > omnibox.MaxQueryBytes || strings.IndexFunc(q, unicode.IsControl) >= 0 {
		return "", ErrInvalid
	}
	escaped := url.QueryEscape(q)
	u, _ := url.Parse(strings.ReplaceAll(t, "{searchTerms}", "GROWSETERMS"))
	if strings.Contains(u.Path, "GROWSETERMS") {
		escaped = url.PathEscape(q)
	}
	return strings.Replace(t, "{searchTerms}", escaped, 1), nil
}
func (p Provider) SearchURL(q string) (string, error) {
	if p.Validate() != nil || p.Disabled {
		return "", ErrInvalid
	}
	return expand(p.SearchTemplate, q)
}
func (s Settings) Validate() error {
	if len(s.Providers) == 0 || len(s.Providers) > MaxProviders {
		return ErrInvalid
	}
	ids, keywords := map[string]bool{}, map[string]bool{}
	found, builtin := false, false
	for index, p := range s.Providers {
		for _, previous := range s.Providers[:index] {
			if strings.EqualFold(previous.Keyword, p.Keyword) {
				return ErrInvalid
			}
		}
		if p.Validate() != nil || ids[p.ID] || keywords[strings.ToLower(p.Keyword)] {
			return ErrInvalid
		}
		ids[p.ID] = true
		keywords[strings.ToLower(p.Keyword)] = true
		if p.ID == s.DefaultID && !p.Disabled {
			found = true
		}
		if p.ID == "duckduckgo" {
			b := Builtin()
			builtin = p.Name == b.Name && p.Keyword == b.Keyword && p.SearchTemplate == b.SearchTemplate && p.SuggestionTemplate == b.SuggestionTemplate
		}
	}
	if !found || !builtin {
		return ErrInvalid
	}
	return nil
}
func (s Settings) Default() Provider {
	for _, p := range s.Providers {
		if p.ID == s.DefaultID && !p.Disabled {
			return p
		}
	}
	return Builtin()
}

// Resolve performs a one-submission keyword override without changing settings.
func (s Settings) Resolve(input string) (Provider, string, bool) {
	input = strings.TrimSpace(input)
	first, rest, ok := strings.Cut(input, " ")
	if ok {
		for _, p := range s.Providers {
			if !p.Disabled && strings.EqualFold(first, p.Keyword) {
				return p, strings.TrimSpace(rest), true
			}
		}
	}
	return s.Default(), input, false
}
func (s *Settings) Put(p Provider) error {
	next := s.Clone()
	found := false
	for i, old := range next.Providers {
		if old.ID == p.ID {
			next.Providers[i] = p
			found = true
			break
		}
	}
	if !found {
		next.Providers = append(next.Providers, p)
	}
	if next.Validate() != nil {
		return ErrInvalid
	}
	*s = next
	return nil
}
func (s *Settings) Delete(id string) error {
	if id == "duckduckgo" {
		return ErrInvalid
	}
	next := s.Clone()
	found := false
	for i, p := range next.Providers {
		if p.ID == id {
			next.Providers = append(next.Providers[:i], next.Providers[i+1:]...)
			found = true
			break
		}
	}
	if next.DefaultID == id {
		next.DefaultID = "duckduckgo"
	}
	if !found || next.Validate() != nil {
		return ErrInvalid
	}
	*s = next
	return nil
}
