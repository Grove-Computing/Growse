package securityscan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Finding struct {
	Path    string
	Line    int
	Column  int
	Title   string
	Message string
}

type Scanner struct {
	Root   string
	Policy Policy
}

func GitFiles(root, diffBase string) ([]string, error) {
	args := []string{"-C", root}
	if diffBase == "" {
		args = append(args, "ls-files", "-z")
	} else {
		args = append(args, "diff", "--name-only", "--diff-filter=ACMR", "-z", diffBase+"...HEAD", "--")
	}
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("list tracked files: %w", err)
	}
	return parseNULList(output), nil
}

func parseNULList(data []byte) []string {
	parts := bytes.Split(data, []byte{0})
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			files = append(files, string(part))
		}
	}
	sort.Strings(files)
	return files
}

func (s Scanner) Scan(paths []string) ([]Finding, error) {
	var findings []Finding
	for _, path := range paths {
		path = filepath.ToSlash(filepath.Clean(path))
		fullPath := filepath.Join(s.Root, filepath.FromSlash(path))
		info, err := os.Lstat(fullPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect %s: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			findings = append(findings, Finding{Path: path, Line: 1, Column: 1, Title: "Unapproved artifact", Message: "symbolic link is not allowed"})
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		if isBinary(path, data) {
			if finding := s.checkBinary(path, data); finding != nil {
				findings = append(findings, *finding)
			}
			continue
		}
		findings = append(findings, s.scanUnicode(path, data)...)
		if path == ".vscode/extensions.json" {
			findings = append(findings, s.scanEditorRecommendations(path, data)...)
		}
	}
	return findings, nil
}

func isBinary(path string, data []byte) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".vsix", ".wasm":
		return true
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return true
	}
	return bytes.HasPrefix(data, []byte("\x7fELF")) ||
		bytes.HasPrefix(data, []byte("MZ")) ||
		bytes.HasPrefix(data, []byte("\x00asm")) ||
		bytes.HasPrefix(data, []byte("PK\x03\x04")) ||
		bytes.HasPrefix(data, []byte("\xfe\xed\xfa")) ||
		bytes.HasPrefix(data, []byte("\xcf\xfa\xed\xfe"))
}

func (s Scanner) checkBinary(path string, data []byte) *Finding {
	digest := sha256.Sum256(data)
	actual := hex.EncodeToString(digest[:])
	for _, allowance := range s.Policy.BinaryAllowlist {
		if allowance.Path == path && allowance.SHA256 == actual {
			return nil
		}
	}
	return &Finding{
		Path:    path,
		Line:    1,
		Column:  1,
		Title:   "Unapproved binary artifact",
		Message: fmt.Sprintf("binary, archive, WASM, or native executable is not hash-allowlisted (sha256:%s)", actual),
	}
}

func (s Scanner) scanUnicode(path string, data []byte) []Finding {
	runes := []rune(string(data))
	line, column := 1, 1
	var findings []Finding
	for index, current := range runes {
		previous, next := rune(0), rune(0)
		if index > 0 {
			previous = runes[index-1]
		}
		if index+1 < len(runes) {
			next = runes[index+1]
		}
		kind, suspicious := suspiciousUnicode(current, previous, next)
		if suspicious && !s.unicodeAllowed(path, current) {
			findings = append(findings, Finding{
				Path:    path,
				Line:    line,
				Column:  column,
				Title:   "Suspicious Unicode",
				Message: fmt.Sprintf("%s U+%04X is not allowed", kind, current),
			})
		}
		if current == '\n' {
			line, column = line+1, 1
		} else {
			column++
		}
	}
	return findings
}

func suspiciousUnicode(current, previous, next rune) (string, bool) {
	switch {
	case current >= 0x202A && current <= 0x202E || current >= 0x2066 && current <= 0x2069:
		return "bidirectional control", true
	case current >= 0xE000 && current <= 0xF8FF || current >= 0xF0000 && current <= 0xFFFFD || current >= 0x100000 && current <= 0x10FFFD:
		return "Private Use Area code point", true
	case current >= 0xE0100 && current <= 0xE01EF:
		return "variation selector", true
	case current >= 0xFE00 && current <= 0xFE0F:
		if (current == 0xFE0E || current == 0xFE0F) && isEmojiBase(previous) {
			return "", false
		}
		return "variation selector", true
	case current == 0x200D && (isEmojiBase(previous) || previous == 0xFE0F) && isEmojiBase(next):
		return "", false
	case unicode.Is(unicode.Cf, current):
		return "zero-width or format control", true
	default:
		return "", false
	}
}

func isEmojiBase(value rune) bool {
	switch {
	case value == 0x00A9 || value == 0x00AE || value == 0x203C || value == 0x2049 || value == 0x2122 || value == 0x2139:
		return true
	case value >= 0x2194 && value <= 0x2199 || value >= 0x21A9 && value <= 0x21AA:
		return true
	case value >= 0x231A && value <= 0x231B || value == 0x2328 || value == 0x23CF || value >= 0x23E9 && value <= 0x23FA:
		return true
	case value == 0x24C2 || value >= 0x25AA && value <= 0x25AB || value == 0x25B6 || value == 0x25C0 || value >= 0x25FB && value <= 0x25FE:
		return true
	case value >= 0x2600 && value <= 0x27BF || value >= 0x2934 && value <= 0x2935 || value >= 0x2B05 && value <= 0x2B07:
		return true
	case value >= 0x2B1B && value <= 0x2B1C || value == 0x2B50 || value == 0x2B55 || value == 0x3030 || value == 0x303D:
		return true
	case value == 0x3297 || value == 0x3299 || value >= 0x1F000 && value <= 0x1FAFF:
		return true
	default:
		return false
	}
}

func (s Scanner) unicodeAllowed(path string, codePoint rune) bool {
	want := fmt.Sprintf("U+%04X", codePoint)
	for _, allowance := range s.Policy.UnicodeAllowlist {
		if allowance.Path == path && strings.EqualFold(allowance.CodePoint, want) {
			return true
		}
	}
	return false
}

func (s Scanner) scanEditorRecommendations(path string, data []byte) []Finding {
	var extensions struct {
		Recommendations []string `json:"recommendations"`
	}
	if err := json.Unmarshal(data, &extensions); err != nil {
		return []Finding{{Path: path, Line: 1, Column: 1, Title: "Invalid editor recommendations", Message: err.Error()}}
	}
	allowed := make(map[string]bool)
	for _, extension := range s.Policy.EditorExtensions {
		allowed[strings.ToLower(extension.ID)] = true
	}
	var findings []Finding
	for _, recommendation := range extensions.Recommendations {
		if !allowed[strings.ToLower(recommendation)] {
			findings = append(findings, Finding{
				Path: path, Line: 1, Column: 1, Title: "Unapproved editor extension",
				Message: fmt.Sprintf("extension %q is not in the approved publisher/version policy", recommendation),
			})
		}
	}
	return findings
}

func FormatFinding(f Finding, githubActions bool) string {
	if !githubActions {
		return fmt.Sprintf("%s:%d:%d: %s: %s", f.Path, f.Line, f.Column, f.Title, f.Message)
	}
	return fmt.Sprintf("::error file=%s,line=%d,col=%d,title=%s::%s",
		escapeProperty(f.Path), f.Line, f.Column, escapeProperty(f.Title), escapeMessage(f.Message))
}

func escapeProperty(value string) string {
	replacer := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
	return replacer.Replace(value)
}

func escapeMessage(value string) string {
	replacer := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	return replacer.Replace(value)
}
