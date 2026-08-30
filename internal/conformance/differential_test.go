package conformance

import "testing"

func TestCompareChecksDOMComputedGeometryScrollFocusAndResources(t *testing.T) {
	reference := Snapshot{
		Scenario: "next-desktop-hydrated", Viewport: Extent{Width: 1280, Height: 720}, DOMLandmarks: []string{"app", "nav"},
		Computed: map[string]map[string]string{"app": {"display": "grid", "color": "rgb(15, 23, 42)"}},
		Geometry: map[string]Rect{"app": {X: 32, Y: 16, Width: 1216, Height: 640}}, ScrollExtent: Extent{Width: 1280, Height: 1180},
		Focus: "filter", Resources: map[string]string{"app.css": "complete", "hero.webp": "complete"},
	}
	actual := reference
	actual.DOMLandmarks = append([]string(nil), reference.DOMLandmarks...)
	actual.Computed = map[string]map[string]string{"app": {"display": "grid", "color": "rgb(15, 23, 42)"}}
	actual.Geometry = map[string]Rect{"app": {X: 33.9, Y: 14, Width: 1228, Height: 646.4}}
	actual.Resources = map[string]string{"app.css": "complete", "hero.webp": "complete"}
	if report := Compare(reference, actual); !report.Passed() {
		t.Fatalf("threshold-compatible snapshot failed: %+v", report.Differences)
	}

	actual.Focus = ""
	actual.Computed["app"]["display"] = "block"
	actual.Resources["hero.webp"] = "pending"
	actual.Geometry["app"] = Rect{X: 36, Y: 16, Width: 1216, Height: 640}
	report := Compare(reference, actual)
	if report.Passed() || !hasDifference(report, "focus") || !hasDifference(report, "computed/app") || !hasDifference(report, "resource") || !hasDifference(report, "geometry/app") {
		t.Fatalf("missing differential categories: %+v", report.Differences)
	}
}

func TestCompareRejectsMissingAndNonFiniteGeometry(t *testing.T) {
	reference := Snapshot{Scenario: "svelte-narrow-ssr", Geometry: map[string]Rect{"main": {Width: 360, Height: 800}}}
	actual := Snapshot{Scenario: reference.Scenario, Geometry: map[string]Rect{}}
	if report := Compare(reference, actual); report.Passed() || !hasDifference(report, "geometry") {
		t.Fatalf("missing geometry passed: %+v", report)
	}
}

func TestGeometryUsesTwoPixelsOrOnePercentAndSemanticStateIsExact(t *testing.T) {
	reference := Snapshot{
		Scenario: "threshold", DOMLandmarks: []string{"small", "large"},
		Geometry: map[string]Rect{
			"small": {X: 10, Y: 10, Width: 100, Height: 100},
			"large": {X: 1000, Y: 1000, Width: 1000, Height: 1000},
		},
		Computed:  map[string]map[string]string{"small": {"display": "block"}},
		Resources: map[string]string{"hero": "complete"}, Focus: "small",
	}
	actual := reference
	actual.DOMLandmarks = append([]string(nil), reference.DOMLandmarks...)
	actual.Geometry = map[string]Rect{
		"small": {X: 12, Y: 8, Width: 102, Height: 98},
		"large": {X: 1010, Y: 990, Width: 1010, Height: 990},
	}
	actual.Computed = map[string]map[string]string{"small": {"display": "block"}}
	actual.Resources = map[string]string{"hero": "complete"}
	if report := Compare(reference, actual); !report.Passed() {
		t.Fatalf("boundary values failed: %+v", report.Differences)
	}

	small := actual.Geometry["small"]
	small.X = 12.01
	actual.Geometry["small"] = small
	if report := Compare(reference, actual); report.Passed() || !hasDifference(report, "geometry/small") {
		t.Fatalf("geometry above 2px passed: %+v", report.Differences)
	}
	small.X = 12
	actual.Geometry["small"] = small
	actual.Computed["small"]["display"] = "inline"
	if report := Compare(reference, actual); report.Passed() || !hasDifference(report, "computed/small") {
		t.Fatalf("non-exact computed state passed: %+v", report.Differences)
	}
}

func hasDifference(report Report, category string) bool {
	for _, difference := range report.Differences {
		if difference.Category == category {
			return true
		}
	}
	return false
}
