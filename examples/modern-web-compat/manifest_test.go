package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type fixtureManifest struct {
	SchemaVersion          int               `json:"schemaVersion"`
	Release                string            `json:"release"`
	ArtifactPolicy         string            `json:"artifactPolicy"`
	PublicInternetRequired bool              `json:"publicInternetRequired"`
	VerificationCommand    string            `json:"verificationCommand"`
	Fixtures               []fixtureManifest `json:"fixtures"`
	Name                   string            `json:"name"`
	Framework              string            `json:"framework"`
	FrameworkVersion       string            `json:"frameworkVersion"`
	BuildTool              string            `json:"buildTool"`
	Dependencies           map[string]string `json:"dependencies"`
	GenerationCommand      string            `json:"generationCommand"`
	Licenses               []string          `json:"licenses"`
	Sources                []string          `json:"sources"`
	Artifacts              []fixtureArtifact `json:"artifacts"`
}

type fixtureArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

var semanticVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var publicResourceReference = regexp.MustCompile(`(?i)(?:src|href)=["']https?://|url\(\s*["']?https?://`)

func TestFixtureManifestIsCompleteAndUntampered(t *testing.T) {
	attributes, err := os.ReadFile("../../.gitattributes")
	if err != nil || !strings.Contains(string(attributes), "examples/modern-web-compat/** text eol=lf") {
		t.Fatalf("fixture checkout is not LF-pinned: %v", err)
	}
	encoded, err := os.ReadFile("fixture-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest fixtureManifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 || manifest.Release != "v0.17.0" || manifest.PublicInternetRequired || manifest.ArtifactPolicy != "checked-in deterministic build artifacts" {
		t.Fatalf("offline fixture manifest header = %#v", manifest)
	}
	if manifest.VerificationCommand == "" || len(manifest.Fixtures) != 4 {
		t.Fatalf("fixture manifest verification/count = %q / %d", manifest.VerificationCommand, len(manifest.Fixtures))
	}

	seen := make(map[string]bool)
	for _, fixture := range manifest.Fixtures {
		if seen[fixture.Name] || fixture.Framework == "" || !semanticVersion.MatchString(fixture.FrameworkVersion) {
			t.Fatalf("invalid fixture identity/version = %#v", fixture)
		}
		seen[fixture.Name] = true
		if fixture.BuildTool == "" || len(fixture.Dependencies) == 0 || fixture.GenerationCommand == "" || len(fixture.Licenses) == 0 {
			t.Fatalf("incomplete provenance for %s", fixture.Name)
		}
		if fixture.Name == "real-site" && !strings.Contains(fixture.GenerationCommand, "--offline") {
			t.Fatalf("real-site generation is not offline reproducible: %q", fixture.GenerationCommand)
		}
		for _, license := range fixture.Licenses {
			if !strings.HasPrefix(license, "MIT:") {
				t.Fatalf("fixture %s license %q is not SPDX-pinned", fixture.Name, license)
			}
		}
		if len(fixture.Sources) == 0 || len(fixture.Artifacts) == 0 {
			t.Fatalf("fixture %s has no source/artifact", fixture.Name)
		}
		for _, source := range fixture.Sources {
			assertLocalFixturePath(t, source, "sources/")
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("fixture %s source %q: %v", fixture.Name, source, err)
			}
		}
		for _, artifact := range fixture.Artifacts {
			assertLocalFixturePath(t, artifact.Path, "fixtures/"+fixture.Name+"/")
			content, err := os.ReadFile(artifact.Path)
			if err != nil {
				t.Fatalf("fixture %s artifact %q: %v", fixture.Name, artifact.Path, err)
			}
			digest := sha256.Sum256(content)
			if got := hex.EncodeToString(digest[:]); got != artifact.SHA256 {
				t.Fatalf("fixture %s artifact %q digest = %s, want %s", fixture.Name, artifact.Path, got, artifact.SHA256)
			}
			if publicResourceReference.Match(content) {
				t.Fatalf("fixture %s artifact %q requires a public URL", fixture.Name, artifact.Path)
			}
		}
	}
	for _, name := range []string{"nextjs", "sveltekit", "tailwind", "real-site"} {
		if !seen[name] {
			t.Fatalf("fixture manifest is missing %s", name)
		}
	}
}

func TestUpstreamFrameworkBuildDigests(t *testing.T) {
	for _, fixture := range []string{"nextjs", "sveltekit"} {
		checksumPath := filepath.Join("fixtures", fixture, "upstream.sha256")
		encoded, err := os.ReadFile(checksumPath)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(encoded)), "\n")
		if len(lines) < 2 {
			t.Fatalf("%s does not cover an upstream module graph", checksumPath)
		}
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) != 2 || len(fields[0]) != 64 {
				t.Fatalf("invalid checksum entry %q", line)
			}
			assertLocalFixturePath(t, fields[1], "upstream-export/")
			content, err := os.ReadFile(filepath.Join("fixtures", fixture, fields[1]))
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(content)
			if got := hex.EncodeToString(digest[:]); got != fields[0] {
				t.Fatalf("fixture %s upstream artifact %q digest = %s, want %s", fixture, fields[1], got, fields[0])
			}
		}
	}
}

func TestRealSiteFixtureContainsGeneratedTailwindAndSSRContracts(t *testing.T) {
	stylesheet, err := os.ReadFile("fixtures/real-site/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tailwindcss v4.1.12", ":root,:host", `.md\:grid-cols-2`, "@keyframes fixture-float", "--spacing-card"} {
		if !strings.Contains(string(stylesheet), want) {
			t.Fatalf("generated Tailwind artifact is missing %q", want)
		}
	}
	html, err := os.ReadFile("fixtures/real-site/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"脆弱性報告", "data-ssr-token=", "md:grid-cols-2", "/assets/pixel.png", "animate-fixture-float", "/real-site/app.mjs"} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("real-site SSR artifact is missing %q", want)
		}
	}
	for _, path := range []string{"package.json", "pnpm-lock.yaml"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("offline toolchain file %s: %v", path, err)
		}
	}
}

func TestFixtureServerUsesPlatformIndependentContentTypes(t *testing.T) {
	server := httptest.NewServer(modernWebCompatibilityHandler())
	defer server.Close()
	for path, want := range map[string]string{
		"/_next/static/chunks/app.mjs":    "text/javascript; charset=utf-8",
		"/_app/immutable/entry/start.mjs": "text/javascript; charset=utf-8",
		"/diagnostics/failures.mjs":       "text/javascript; charset=utf-8",
		"/_next/static/css/app.css":       "text/css; charset=utf-8",
		"/_app/immutable/assets/app.css":  "text/css; charset=utf-8",
		"/real-site/app.css":              "text/css; charset=utf-8",
		"/real-site/app.mjs":              "text/javascript; charset=utf-8",
	} {
		response, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if got := response.Header.Get("Content-Type"); got != want {
			t.Errorf("%s Content-Type = %q, want %q", path, got, want)
		}
	}
}

func assertLocalFixturePath(t *testing.T, path, prefix string) {
	t.Helper()
	clean := filepath.ToSlash(filepath.Clean(path))
	if filepath.IsAbs(path) || clean != path || !strings.HasPrefix(clean, prefix) || strings.Contains(clean, "../") {
		t.Fatalf("fixture path %q escapes local prefix %q", path, prefix)
	}
}
