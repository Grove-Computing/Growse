package browser

import "net/url"

// Script is one Go source discovered in document order.
type Script struct {
	SourceURL *url.URL
	Source    string
	Inline    bool
}

// IsTrustedOrigin reports whether automatic Go execution is permitted for u.
func IsTrustedOrigin(u *url.URL) bool {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
