package securityscan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadPolicyRejectsExpiredUnicodeAllowance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	data := `{
  "binary_allowlist": [],
  "unicode_allowlist": [{
    "path": "legacy.txt",
    "code_point": "U+200B",
    "reason": "temporary migration",
    "expires": "2026-08-14"
  }],
  "editor_extensions": []
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadPolicy(path, time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("LoadPolicy() error = %v, want expired", err)
	}
}

func TestLoadPolicyRequiresExtensionVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	data := `{
  "binary_allowlist": [],
  "unicode_allowlist": [],
  "editor_extensions": [{"id":"golang.go","publisher":"Go Team at Google","version":""}]
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadPolicy(path, time.Now())
	if err == nil || !strings.Contains(err.Error(), "editor extension") {
		t.Fatalf("LoadPolicy() error = %v, want editor extension validation", err)
	}
}
