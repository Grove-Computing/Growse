package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/conformance"
)

type compatibilityCorpus struct {
	SchemaVersion int          `json:"schemaVersion"`
	Release       string       `json:"release"`
	Chromium      string       `json:"chromium"`
	Offline       bool         `json:"offline"`
	Pages         []corpusPage `json:"pages"`
}

func TestFixedRunnerPerformanceGateDoesNotRegressFromV016(t *testing.T) {
	encoded, err := os.ReadFile("performance-gate.json")
	if err != nil {
		t.Fatal(err)
	}
	var gate conformance.PerformanceGate
	if err := json.Unmarshal(encoded, &gate); err != nil {
		t.Fatal(err)
	}
	if report := conformance.ComparePerformance(gate); !report.Passed() {
		t.Fatalf("fixed runner performance gate failed: %+v", report.Differences)
	}
}

type corpusPage struct {
	Name              string   `json:"name"`
	Framework         string   `json:"framework"`
	Path              string   `json:"path"`
	Viewports         []string `json:"viewports"`
	DevicePixelRatios []int    `json:"devicePixelRatios"`
	States            []string `json:"states"`
}

func TestBrowserGradeCorpusCoversFrameworkViewportDPRAndLifecycleMatrix(t *testing.T) {
	encoded, err := os.ReadFile("corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus compatibilityCorpus
	if err := json.Unmarshal(encoded, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.SchemaVersion != 1 || corpus.Release != "v0.17.0" || corpus.Chromium == "" || !corpus.Offline || len(corpus.Pages) != 2 {
		t.Fatalf("corpus header = %#v", corpus)
	}
	wantStates := []string{"ssr", "resource-complete", "hydrated", "interaction", "scroll", "animation"}
	for _, page := range corpus.Pages {
		if page.Name == "" || page.Framework == "" || !strings.HasPrefix(page.Path, "/") || !sameStrings(page.Viewports, []string{"desktop", "narrow"}) || !sameInts(page.DevicePixelRatios, []int{1, 2}) || !sameStrings(page.States, wantStates) {
			t.Fatalf("incomplete corpus page = %#v", page)
		}
	}
}

func TestBrowserGradeShowcaseServesPinnedFrameworkArtifacts(t *testing.T) {
	handler := browserGradeCompatibilityHandler("..")
	for _, path := range []string{"/showcase/", "/next/", "/svelte/", "/tailwind/", "/real-site/", "/diagnostics/"} {
		request := httptest.NewRequest(http.MethodGet, "http://showcase.local"+path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s = status:%d bytes:%d", path, response.Code, response.Body.Len())
		}
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func sameInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
