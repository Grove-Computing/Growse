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
	if manifest.SchemaVersion != 1 || manifest.Release != "v0.15.0" || manifest.PublicInternetRequired {
		t.Fatalf("offline fixture manifest header = %#v", manifest)
	}
	if manifest.VerificationCommand == "" || len(manifest.Fixtures) != 3 {
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
			if strings.Contains(string(content), "https://") || strings.Contains(string(content), "http://") {
				t.Fatalf("fixture %s artifact %q requires a public URL", fixture.Name, artifact.Path)
			}
		}
	}
	for _, name := range []string{"nextjs", "sveltekit", "tailwind"} {
		if !seen[name] {
			t.Fatalf("fixture manifest is missing %s", name)
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
