package network

import (
	"net/http"
	"testing"
	"time"
)

// RFC 9111 sections 4.2.1 and 4.2.3: freshness lifetime and current age.
func TestRFC9111FreshnessAndAgeExamples(t *testing.T) {
	storedAt := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		header     http.Header
		lifetime   time.Duration
		initialAge time.Duration
	}{
		{
			name: "max-age overrides an already expired Expires",
			header: http.Header{
				"Date": []string{storedAt.Format(http.TimeFormat)}, "Cache-Control": []string{"max-age=60"},
				"Expires": []string{storedAt.Add(-time.Hour).Format(http.TimeFormat)},
			},
			lifetime: time.Minute,
		},
		{
			name: "Expires supplies freshness without max-age",
			header: http.Header{
				"Date": []string{storedAt.Format(http.TimeFormat)}, "Expires": []string{storedAt.Add(2 * time.Minute).Format(http.TimeFormat)},
			},
			lifetime: 2 * time.Minute,
		},
		{
			name: "apparent age is greater than Age header",
			header: http.Header{
				"Date": []string{storedAt.Add(-30 * time.Second).Format(http.TimeFormat)},
				"Age":  []string{"20"}, "Cache-Control": []string{"max-age=60"},
			},
			lifetime: time.Minute, initialAge: 30 * time.Second,
		},
		{
			name: "Age greater than lifetime is stale immediately",
			header: http.Header{
				"Date": []string{storedAt.Format(http.TimeFormat)}, "Age": []string{"120"}, "Cache-Control": []string{"max-age=60"},
			},
			lifetime: time.Minute, initialAge: 2 * time.Minute,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := calculateFreshness(test.header, storedAt)
			if got.lifetime != test.lifetime || got.initialAge != test.initialAge {
				t.Fatalf("freshness = lifetime:%v age:%v, want %v/%v", got.lifetime, got.initialAge, test.lifetime, test.initialAge)
			}
			if test.initialAge >= test.lifetime && got.fresh(storedAt) {
				t.Fatal("response older than its lifetime was fresh")
			}
		})
	}
}
