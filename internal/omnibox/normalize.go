package omnibox

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

// NormalizeURL defines candidate identity without rewriting navigation targets.
// It removes fragments, default ports and a trailing slash, and canonicalizes
// hosts. Scheme, escaped path and the original query remain distinct.
func NormalizeURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid candidate URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "http" && scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return "", fmt.Errorf("invalid candidate URL")
	}
	host := strings.ToLower(u.Hostname())
	if addr, err := netip.ParseAddr(host); err == nil {
		host = addr.String()
	} else {
		host, err = idna.Lookup.ToASCII(host)
		if err != nil {
			return "", fmt.Errorf("invalid candidate host")
		}
	}
	port := u.Port()
	if port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", fmt.Errorf("invalid candidate port")
		}
		port = strconv.Itoa(number)
	}
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	key := scheme + "://" + host + path
	if u.RawQuery != "" || u.ForceQuery {
		key += "?" + u.RawQuery
	}
	return key, nil
}

func candidateKey(c Candidate) string {
	if c.URL != "" {
		if key, err := NormalizeURL(c.URL); err == nil {
			return "url:" + key
		}
	}
	if c.Source == TabSource {
		return "tab:" + strconv.FormatUint(c.TabID, 10)
	}
	return "query:" + normalized(c.Query)
}

func sourcePriority(c Candidate) int {
	switch c.Source {
	case TabSource:
		return 5
	case BookmarkSource:
		return 4
	case HistorySource:
		if c.TypedCount > 0 {
			return 3
		}
		return 2
	case InputSource:
		return 1
	default:
		return 0
	}
}

func preferredCandidate(a, b Candidate) bool {
	if sourcePriority(a) != sourcePriority(b) {
		return sourcePriority(a) > sourcePriority(b)
	}
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.URL != b.URL {
		return a.URL < b.URL
	}
	if a.Primary != b.Primary {
		return a.Primary < b.Primary
	}
	return a.TabID < b.TabID
}

func deduplicate(candidates []Candidate) []Candidate {
	indices := make(map[string]int, len(candidates))
	result := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		key := candidateKey(c)
		if index, exists := indices[key]; exists {
			if preferredCandidate(c, result[index]) {
				result[index] = c
			}
		} else {
			indices[key] = len(result)
			result = append(result, c)
		}
	}
	return result
}
