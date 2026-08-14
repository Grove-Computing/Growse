package securityscan

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Policy struct {
	BinaryAllowlist  []BinaryAllowance  `json:"binary_allowlist"`
	UnicodeAllowlist []UnicodeAllowance `json:"unicode_allowlist"`
	EditorExtensions []EditorExtension  `json:"editor_extensions"`
}

type BinaryAllowance struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Reason string `json:"reason"`
}

type UnicodeAllowance struct {
	Path      string `json:"path"`
	CodePoint string `json:"code_point"`
	Reason    string `json:"reason"`
	Expires   string `json:"expires"`
}

type EditorExtension struct {
	ID        string `json:"id"`
	Publisher string `json:"publisher"`
	Version   string `json:"version"`
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var codePointPattern = regexp.MustCompile(`^U\+[0-9A-Fa-f]{4,6}$`)

func LoadPolicy(path string, now time.Time) (Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("read policy: %w", err)
	}
	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return Policy{}, fmt.Errorf("decode policy: %w", err)
	}
	if err := policy.validate(now); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

func (p Policy) validate(now time.Time) error {
	seenBinary := make(map[string]bool)
	for _, allowance := range p.BinaryAllowlist {
		if allowance.Path == "" || strings.TrimSpace(allowance.Reason) == "" || !sha256Pattern.MatchString(allowance.SHA256) {
			return fmt.Errorf("invalid binary allowlist entry for %q", allowance.Path)
		}
		if seenBinary[allowance.Path] {
			return fmt.Errorf("duplicate binary allowlist entry for %q", allowance.Path)
		}
		seenBinary[allowance.Path] = true
	}

	for _, allowance := range p.UnicodeAllowlist {
		if allowance.Path == "" || strings.TrimSpace(allowance.Reason) == "" || !validCodePoint(allowance.CodePoint) {
			return fmt.Errorf("invalid Unicode allowlist entry for %q", allowance.Path)
		}
		expires, err := time.Parse(time.DateOnly, allowance.Expires)
		if err != nil {
			return fmt.Errorf("invalid Unicode allowlist expiration for %q: %w", allowance.Path, err)
		}
		if !expires.After(dateOnly(now)) {
			return fmt.Errorf("unicode allowlist entry expired for %q on %s", allowance.Path, allowance.Expires)
		}
	}

	seenExtension := make(map[string]bool)
	for _, extension := range p.EditorExtensions {
		id := strings.ToLower(extension.ID)
		if id == "" || !strings.Contains(id, ".") || strings.TrimSpace(extension.Publisher) == "" || strings.TrimSpace(extension.Version) == "" {
			return fmt.Errorf("invalid editor extension policy for %q", extension.ID)
		}
		if seenExtension[id] {
			return fmt.Errorf("duplicate editor extension policy for %q", extension.ID)
		}
		seenExtension[id] = true
	}
	return nil
}

func validCodePoint(value string) bool {
	if !codePointPattern.MatchString(value) {
		return false
	}
	codePoint, err := strconv.ParseInt(value[2:], 16, 32)
	if err != nil {
		return false
	}
	return codePoint >= 0 && codePoint <= 0x10FFFF
}

func dateOnly(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
