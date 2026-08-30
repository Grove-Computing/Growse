package browser

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestImageResourcePriorityCombinesPreloadFetchPriorityLazyAndViewport(t *testing.T) {
	document := dom.NewDocument()
	preload := document.CreateElement("link", map[string]string{"rel": "stylesheet PRELOAD", "as": "image", "href": "/preloaded.png"})
	if err := document.AppendChild(document.Root, preload); err != nil {
		t.Fatal(err)
	}
	baseURL := mustParseURL(t, "https://example.com/page/")
	preloads := imagePreloads(document, baseURL)
	tests := []struct {
		attributes map[string]string
		target     string
		eligible   bool
		want       resourcePriority
	}{
		{map[string]string{"fetchpriority": "high"}, "/hero.png", false, resourcePriorityCritical},
		{nil, "/preloaded.png", false, resourcePriorityCritical},
		{nil, "/near.png", true, resourcePriorityHigh},
		{nil, "/normal.png", false, resourcePriorityNormal},
		{map[string]string{"loading": "lazy"}, "/far.png", false, resourcePriorityLow},
		{map[string]string{"fetchpriority": "low"}, "/low.png", true, resourcePriorityLow},
	}
	for _, test := range tests {
		node := document.CreateElement("img", test.attributes)
		target := resolveImageCandidate(baseURL, test.target)
		if got := imageResourcePriority(node, target, test.eligible, preloads); got != test.want {
			t.Fatalf("priority(%v, %s, %v) = %d, want %d", test.attributes, test.target, test.eligible, got, test.want)
		}
	}
}

func TestResourceQueueBoundsConcurrencyPendingWorkAndPreservesPriority(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	release := make(chan struct{})
	started := make(chan int, maxResourceQueue)
	jobs := make([]resourceJob, maxResourceQueue+9)
	for index := range jobs {
		index := index
		priority := resourcePriorityLow
		if index == len(jobs)-1 {
			priority = resourcePriorityCritical
		}
		jobs[index] = resourceJob{priority: priority, order: index, run: func(context.Context) {
			current := active.Add(1)
			for current > maximum.Load() && !maximum.CompareAndSwap(maximum.Load(), current) {
			}
			started <- index
			<-release
			active.Add(-1)
		}}
	}
	done := make(chan int, 1)
	go func() { done <- runBoundedResourceJobs(context.Background(), jobs) }()
	first := make([]int, 0, maxResourceWorkers)
	for range maxResourceWorkers {
		select {
		case index := <-started:
			first = append(first, index)
		case <-time.After(time.Second):
			t.Fatal("resource workers did not start")
		}
	}
	if maximum.Load() > maxResourceWorkers {
		t.Fatalf("maximum concurrency = %d", maximum.Load())
	}
	close(release)
	if rejected := <-done; rejected != 9 {
		t.Fatalf("rejected jobs = %d, want 9", rejected)
	}
	close(started)
	seenCritical := false
	for _, index := range first {
		seenCritical = seenCritical || index == len(jobs)-1
	}
	for index := range started {
		if index == len(jobs)-1 {
			seenCritical = true
		}
	}
	if !seenCritical {
		t.Fatal("critical resource was rejected behind low-priority work")
	}
}
