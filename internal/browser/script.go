package browser

import (
	"net/url"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

// Script is one Go source discovered in document order.
type Script = runtimemodel.Script

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
