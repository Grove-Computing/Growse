// Package conformance compares deterministic browser snapshots without
// retaining document, network body, image, or credential payloads.
package conformance

import (
	"fmt"
	"math"
	"sort"
)

const (
	DefaultGeometryPixels  = 2.0
	DefaultGeometryPercent = 0.01
)

type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Extent struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Snapshot is the body-free state captured from Chromium or Growse at one
// corpus checkpoint.
type Snapshot struct {
	Scenario     string                       `json:"scenario"`
	Viewport     Extent                       `json:"viewport"`
	DOMLandmarks []string                     `json:"domLandmarks"`
	Computed     map[string]map[string]string `json:"computed"`
	Geometry     map[string]Rect              `json:"geometry"`
	ScrollExtent Extent                       `json:"scrollExtent"`
	Focus        string                       `json:"focus"`
	Resources    map[string]string            `json:"resources"`
}

type Difference struct {
	Category string `json:"category"`
	Subject  string `json:"subject"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

type Report struct {
	Scenario    string       `json:"scenario"`
	Differences []Difference `json:"differences,omitempty"`
}

func (report Report) Passed() bool { return len(report.Differences) == 0 }

// Compare applies the v0.17.0 semantic and geometry release thresholds.
func Compare(reference, actual Snapshot) Report {
	report := Report{Scenario: reference.Scenario}
	if reference.Scenario != actual.Scenario {
		report.add("scenario", "name", reference.Scenario, actual.Scenario)
	}
	compareStringSets(&report, "dom", reference.DOMLandmarks, actual.DOMLandmarks)
	compareNestedStrings(&report, "computed", reference.Computed, actual.Computed)
	compareStrings(&report, "resource", reference.Resources, actual.Resources)
	if reference.Focus != actual.Focus {
		report.add("focus", "active-element", reference.Focus, actual.Focus)
	}
	compareExtent(&report, "scroll", reference.ScrollExtent, actual.ScrollExtent)
	compareGeometry(&report, reference, actual)
	return report
}

func (report *Report) add(category, subject, expected, actual string) {
	report.Differences = append(report.Differences, Difference{Category: category, Subject: subject, Expected: expected, Actual: actual})
}

func compareStringSets(report *Report, category string, reference, actual []string) {
	want, got := append([]string(nil), reference...), append([]string(nil), actual...)
	sort.Strings(want)
	sort.Strings(got)
	if fmt.Sprint(want) != fmt.Sprint(got) {
		report.add(category, "landmarks", fmt.Sprint(want), fmt.Sprint(got))
	}
}

func compareNestedStrings(report *Report, category string, reference, actual map[string]map[string]string) {
	for _, landmark := range unionKeys(reference, actual) {
		want, wantExists := reference[landmark]
		got, gotExists := actual[landmark]
		if !wantExists || !gotExists {
			report.add(category, landmark, fmt.Sprint(want), fmt.Sprint(got))
			continue
		}
		compareStrings(report, category+"/"+landmark, want, got)
	}
}

func compareStrings(report *Report, category string, reference, actual map[string]string) {
	for _, key := range unionKeys(reference, actual) {
		want, wantExists := reference[key]
		got, gotExists := actual[key]
		if !wantExists || !gotExists || want != got {
			report.add(category, key, want, got)
		}
	}
}

func compareExtent(report *Report, category string, reference, actual Extent) {
	for _, value := range []struct {
		name      string
		reference float64
		actual    float64
	}{
		{"width", reference.Width, actual.Width},
		{"height", reference.Height, actual.Height},
	} {
		if !withinGeometryThreshold(value.reference, value.actual) {
			report.add(category, value.name, number(value.reference), number(value.actual))
		}
	}
}

func compareGeometry(report *Report, reference, actual Snapshot) {
	for _, landmark := range unionKeys(reference.Geometry, actual.Geometry) {
		want, wantExists := reference.Geometry[landmark]
		got, gotExists := actual.Geometry[landmark]
		if !wantExists || !gotExists {
			report.add("geometry", landmark, fmt.Sprint(want), fmt.Sprint(got))
			continue
		}
		for _, value := range []struct {
			name      string
			reference float64
			actual    float64
		}{
			{"x", want.X, got.X}, {"y", want.Y, got.Y}, {"width", want.Width, got.Width}, {"height", want.Height, got.Height},
		} {
			if !withinGeometryThreshold(value.reference, value.actual) {
				report.add("geometry/"+landmark, value.name, number(value.reference), number(value.actual))
			}
		}
	}
}

func withinGeometryThreshold(reference, actual float64) bool {
	tolerance := math.Max(DefaultGeometryPixels, math.Abs(reference)*DefaultGeometryPercent)
	return !math.IsNaN(actual) && !math.IsInf(actual, 0) && math.Abs(reference-actual) <= tolerance
}

func unionKeys[V any](left, right map[string]V) []string {
	keys := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		keys[key] = struct{}{}
	}
	for key := range right {
		keys[key] = struct{}{}
	}
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func number(value float64) string { return fmt.Sprintf("%.3f", value) }
