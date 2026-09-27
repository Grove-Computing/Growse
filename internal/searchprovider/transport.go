package searchprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Grove-Computing/Growse/internal/omnibox"
)

const MaxDescriptionBytes = 64 * 1024
const MaxSuggestionBytes = 256 * 1024

var ErrResponse = errors.New("検索providerの応答を取得できません")

// Transport copies a client so provider-specific redirects cannot weaken it.
// It sends no page cookies, referer, or authorization headers.
type Transport struct{ client *http.Client }

func NewTransport(client *http.Client) *Transport {
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	copy.Jar = nil
	copy.Timeout = 2 * time.Second
	previous := copy.CheckRedirect
	copy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || validateEndpoint(req.URL) != nil {
			return ErrResponse
		}
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		req.Header.Del("Referer")
		if previous != nil {
			if previous(req, via) != nil {
				return ErrResponse
			}
		}
		return nil
	}
	return &Transport{client: &copy}
}
func (t *Transport) get(ctx context.Context, target string, limit int) ([]byte, error) {
	u, err := url.Parse(target)
	if err != nil || validateEndpoint(u) != nil {
		return nil, ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, ErrResponse
	}
	response, err := t.client.Do(req)
	if err != nil {
		return nil, ErrResponse
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrResponse
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(limit+1)))
	if err != nil || len(body) > limit {
		return nil, ErrResponse
	}
	return body, nil
}

// CanSuggest is stricter than submit classification: URL fragments embedded in
// natural language, credentials and commands must never leave the browser.
func CanSuggest(input string) bool {
	if omnibox.Classify(input).Kind != omnibox.Search {
		return false
	}
	for _, word := range strings.Fields(input) {
		if strings.ContainsAny(word, "@:/\\") || strings.HasPrefix(word, "localhost") || strings.Contains(word, ".") || net.ParseIP(strings.Trim(word, "[]")) != nil {
			return false
		}
	}
	return true
}
func (t *Transport) Suggestions(p Provider) omnibox.RemoteFetcher {
	return func(ctx context.Context, q string) ([]string, error) {
		if p.Validate() != nil || p.Disabled || p.SuggestionTemplate == "" || !CanSuggest(q) {
			return nil, ErrInvalid
		}
		target, err := expand(p.SuggestionTemplate, q)
		if err != nil {
			return nil, err
		}
		body, err := t.get(ctx, target, MaxSuggestionBytes)
		if err != nil {
			return nil, err
		}
		// OpenSearch JSON response [query, [terms], ...]. DuckDuckGo also exposes
		// an array of {phrase: ...}; both are treated strictly as plain text.
		var tuple []json.RawMessage
		var terms []string
		if json.Unmarshal(body, &tuple) == nil && len(tuple) >= 2 && json.Unmarshal(tuple[1], &terms) == nil {
			var echo string
			if json.Unmarshal(tuple[0], &echo) != nil || echo != q {
				return nil, ErrResponse
			}
		} else {
			var records []struct {
				Phrase string `json:"phrase"`
			}
			if json.Unmarshal(body, &records) != nil {
				return nil, ErrResponse
			}
			for _, r := range records {
				terms = append(terms, r.Phrase)
			}
		}
		out := make([]string, 0, min(len(terms), omnibox.MaxSourceCandidates))
		for _, term := range terms {
			if CanSuggest(term) {
				out = append(out, term)
			}
			if len(out) == omnibox.MaxSourceCandidates {
				break
			}
		}
		return out, nil
	}
}
