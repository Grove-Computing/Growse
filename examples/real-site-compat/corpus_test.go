package realsitecompat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type corpusManifest struct {
	SchemaVersion    int    `json:"schemaVersion"`
	Release          string `json:"release"`
	CapturedAt       string `json:"capturedAt"`
	ReferenceBrowser string `json:"referenceBrowser"`
	Offline          bool   `json:"offline"`
	Viewports        []struct {
		Name             string  `json:"name"`
		Width            int     `json:"width"`
		Height           int     `json:"height"`
		DevicePixelRatio float64 `json:"devicePixelRatio"`
	} `json:"viewports"`
	Locale  string   `json:"locale"`
	FontSet []string `json:"fontSet"`
	Limits  struct {
		MaxPixels              int `json:"maxPixels"`
		MaxArtifactBytes       int `json:"maxArtifactBytes"`
		MaxCorpusArtifactBytes int `json:"maxCorpusArtifactBytes"`
		MaxGlyphs              int `json:"maxGlyphs"`
		MaxDisplayedElements   int `json:"maxDisplayedElements"`
		MaxStylesheets         int `json:"maxStylesheets"`
		MaxCSSRules            int `json:"maxCSSRules"`
	} `json:"limits"`
	Cases []struct {
		ID        string   `json:"id"`
		SourceURL string   `json:"sourceURL"`
		Fixture   string   `json:"fixture"`
		SHA256    string   `json:"sha256"`
		Regions   []string `json:"regions"`
	} `json:"cases"`
}

func TestRealSiteCorpusPinsSevenSanitizedOfflineFixtures(t *testing.T) {
	manifest := readCorpusManifest(t)
	if manifest.SchemaVersion != 1 || manifest.Release != "v0.20.0" || manifest.CapturedAt != "2026-09-19" || manifest.ReferenceBrowser == "" || !manifest.Offline {
		t.Fatalf("incomplete corpus header: %#v", manifest)
	}
	if len(manifest.Viewports) != 2 || manifest.Viewports[0].Name != "desktop" || manifest.Viewports[1].Name != "narrow" || manifest.Locale != "ja-JP" || len(manifest.FontSet) < 4 {
		t.Fatalf("incomplete visual environment: %#v", manifest)
	}
	wantIDs := []string{"one-mb-club", "wikipedia-ja", "github-profile", "cv-btxx", "t0-vc", "schemescape", "kidlat"}
	if len(manifest.Cases) != len(wantIDs) {
		t.Fatalf("corpus cases = %d, want %d", len(manifest.Cases), len(wantIDs))
	}
	for index, fixture := range manifest.Cases {
		if fixture.ID != wantIDs[index] || !strings.HasPrefix(fixture.SourceURL, "https://") || len(fixture.Regions) < 2 {
			t.Fatalf("incomplete fixture metadata: %#v", fixture)
		}
		clean := filepath.Clean(fixture.Fixture)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			t.Fatalf("fixture escapes corpus: %q", fixture.Fixture)
		}
		body, err := os.ReadFile(clean)
		if err != nil {
			t.Fatalf("read %s: %v", fixture.ID, err)
		}
		digest := sha256.Sum256(body)
		if got := hex.EncodeToString(digest[:]); got != fixture.SHA256 {
			t.Fatalf("%s sha256 = %s, want %s", fixture.ID, got, fixture.SHA256)
		}
		text := string(body)
		for _, forbidden := range []string{"http://", "src=\"https://", "href=\"https://", "cookie", "authorization", "access_token"} {
			if strings.Contains(strings.ToLower(text), forbidden) {
				t.Fatalf("%s contains non-sanitized external or credential marker %q", fixture.ID, forbidden)
			}
		}
	}
}

func TestRealSiteCorpusLimitsAndProvenanceAreBounded(t *testing.T) {
	manifest := readCorpusManifest(t)
	if manifest.Limits.MaxPixels != 16_000_000 || manifest.Limits.MaxArtifactBytes != 16<<20 || manifest.Limits.MaxCorpusArtifactBytes != 256<<20 || manifest.Limits.MaxGlyphs != 50_000 || manifest.Limits.MaxDisplayedElements != 4_000 || manifest.Limits.MaxStylesheets != 256 || manifest.Limits.MaxCSSRules != 8_192 {
		t.Fatalf("unexpected corpus limits: %#v", manifest.Limits)
	}
	date := regexp.MustCompile(`^20[0-9]{2}-[0-9]{2}-[0-9]{2}$`)
	if !date.MatchString(manifest.CapturedAt) || !strings.HasPrefix(manifest.ReferenceBrowser, "Chromium ") {
		t.Fatalf("invalid provenance: captured=%q browser=%q", manifest.CapturedAt, manifest.ReferenceBrowser)
	}
}

func readCorpusManifest(t *testing.T) corpusManifest {
	t.Helper()
	encoded, err := os.ReadFile("corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest corpusManifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}
