package securityscan

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuspiciousUnicodePositiveFixtures(t *testing.T) {
	tests := []struct {
		name string
		text string
		kind string
	}{
		{name: "bidi override", text: "safe" + string(rune(0x202E)) + "hidden", kind: "bidirectional"},
		{name: "bidi isolate", text: string(rune(0x2066)), kind: "bidirectional"},
		{name: "zero width space", text: "go" + string(rune(0x200B)) + "run", kind: "zero-width"},
		{name: "variation selector", text: "x" + string(rune(0xFE01)), kind: "variation selector"},
		{name: "supplementary variation selector", text: "x" + string(rune(0xE0100)), kind: "variation selector"},
		{name: "private use", text: string(rune(0xE000)), kind: "Private Use Area"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := scanTextFixture(t, tt.text, Policy{})
			if len(findings) != 1 || !strings.Contains(findings[0].Message, tt.kind) {
				t.Fatalf("findings = %#v, want one %q finding", findings, tt.kind)
			}
		})
	}
}

func TestSuspiciousUnicodeNegativeFixture(t *testing.T) {
	text := "通常の日本語、accent café、絵文字 😀 " +
		string(rune(0x1F469)) + string(rune(0x200D)) + string(rune(0x1F4BB)) + " " +
		string(rune(0x2615)) + string(rune(0xFE0F)) + " " +
		string(rune(0x2764)) + string(rune(0xFE0F)) + string(rune(0x200D)) + string(rune(0x1F525))
	if findings := scanTextFixture(t, text, Policy{}); len(findings) != 0 {
		t.Fatalf("ordinary Unicode produced findings: %#v", findings)
	}
}

func TestUnicodeAllowlist(t *testing.T) {
	policy := Policy{UnicodeAllowlist: []UnicodeAllowance{{
		Path: "fixture.txt", CodePoint: "U+200B", Reason: "temporary migration", Expires: "2099-01-01",
	}}}
	if findings := scanTextFixture(t, "a"+string(rune(0x200B))+"b", policy); len(findings) != 0 {
		t.Fatalf("allowlisted Unicode produced findings: %#v", findings)
	}
}

func TestBinaryRequiresMatchingHash(t *testing.T) {
	root := t.TempDir()
	data := []byte{'M', 'Z', 0, 1, 2}
	writeFixture(t, root, "asset.bin", data)

	scanner := Scanner{Root: root}
	findings, err := scanner.Scan([]string{"asset.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Title != "Unapproved binary artifact" {
		t.Fatalf("findings = %#v, want unapproved binary", findings)
	}

	digest := sha256.Sum256(data)
	scanner.Policy.BinaryAllowlist = []BinaryAllowance{{
		Path: "asset.bin", SHA256: hex.EncodeToString(digest[:]), Reason: "test fixture",
	}}
	findings, err = scanner.Scan([]string{"asset.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("hash-allowlisted binary produced findings: %#v", findings)
	}
}

func TestVSIXAndWASMExtensionsAreAlwaysTreatedAsBinary(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "extension.vsix", []byte("otherwise valid UTF-8"))
	writeFixture(t, root, "loader.wasm", []byte("otherwise valid UTF-8"))
	findings, err := (Scanner{Root: root}).Scan([]string{"extension.vsix", "loader.wasm"})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %#v, want both artifacts rejected", findings)
	}
}

func TestEditorRecommendations(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, ".vscode/extensions.json", []byte(`{"recommendations":["golang.go","untrusted.extension"]}`))
	scanner := Scanner{Root: root, Policy: Policy{EditorExtensions: []EditorExtension{{ID: "golang.go"}}}}
	findings, err := scanner.Scan([]string{".vscode/extensions.json"})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "untrusted.extension") {
		t.Fatalf("findings = %#v, want untrusted extension", findings)
	}
}

func TestGitFilesUsesIndex(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeFixture(t, root, "tracked.txt", []byte("tracked"))
	writeFixture(t, root, "untracked.txt", []byte("untracked"))
	runGit(t, root, "add", "tracked.txt")

	files, err := GitFiles(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "tracked.txt" {
		t.Fatalf("GitFiles() = %#v, want tracked file only", files)
	}
}

func TestGitFilesUsesCommittedDiff(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Security Scan Test")
	runGit(t, root, "config", "user.email", "security-scan@example.invalid")
	runGit(t, root, "config", "commit.gpgsign", "false")
	writeFixture(t, root, "changed.txt", []byte("base"))
	runGit(t, root, "add", "changed.txt")
	runGit(t, root, "commit", "-m", "base")
	base := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))

	writeFixture(t, root, "changed.txt", []byte("head"))
	writeFixture(t, root, "added.txt", []byte("added"))
	runGit(t, root, "add", "changed.txt", "added.txt")
	runGit(t, root, "commit", "-m", "head")

	files, err := GitFiles(root, base)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"added.txt", "changed.txt"}
	if strings.Join(files, ",") != strings.Join(want, ",") {
		t.Fatalf("GitFiles(diff) = %#v, want %#v", files, want)
	}
}

func TestFormatFindingAsGitHubAnnotation(t *testing.T) {
	got := FormatFinding(Finding{Path: "a,b.go", Line: 2, Column: 3, Title: "Suspicious Unicode", Message: "U+202E"}, true)
	want := "::error file=a%2Cb.go,line=2,col=3,title=Suspicious Unicode::U+202E"
	if got != want {
		t.Fatalf("FormatFinding() = %q, want %q", got, want)
	}
}

func scanTextFixture(t *testing.T, text string, policy Policy) []Finding {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, root, "fixture.txt", []byte(text))
	findings, err := (Scanner{Root: root, Policy: policy}).Scan([]string{"fixture.txt"})
	if err != nil {
		t.Fatal(err)
	}
	return findings
}

func writeFixture(t *testing.T, root, path string, data []byte) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(output)
}
