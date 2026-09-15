package browser

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
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

func TestScriptResourcePriorityRespectsFetchPriorityWithoutChangingSchedule(t *testing.T) {
	tests := []struct {
		name     string
		schedule runtimemodel.ScriptSchedule
		hint     string
		want     resourcePriority
	}{
		{name: "defer defaults high", schedule: runtimemodel.ScriptDefer, want: resourcePriorityHigh},
		{name: "async defaults normal", schedule: runtimemodel.ScriptAsync, want: resourcePriorityNormal},
		{name: "high is critical", schedule: runtimemodel.ScriptAsync, hint: " HIGH ", want: resourcePriorityCritical},
		{name: "low is low", schedule: runtimemodel.ScriptDefer, hint: "low", want: resourcePriorityLow},
		{name: "unknown preserves fallback", schedule: runtimemodel.ScriptAsync, hint: "urgent", want: resourcePriorityNormal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := scriptSource{schedule: test.schedule, fetchPriority: test.hint}
			if got := scriptResourcePriority(candidate); got != test.want {
				t.Fatalf("script resource priority = %d, want %d", got, test.want)
			}
			if candidate.schedule != test.schedule {
				t.Fatalf("script schedule changed to %q", candidate.schedule)
			}
		})
	}
}

func TestImagePreloadPriorityAppliesToBackgroundResource(t *testing.T) {
	document := dom.NewDocument()
	preload := document.CreateElement("link", map[string]string{
		"rel": "preload", "as": "image", "href": "/hero-background.png", "fetchpriority": "low",
	})
	if err := document.AppendChild(document.Root, preload); err != nil {
		t.Fatal(err)
	}
	baseURL := mustParseURL(t, "https://example.com/page/")
	priorities := imagePreloadPriorities(document, baseURL)
	target := "https://example.com/hero-background.png"
	if got := priorities[target]; got != resourcePriorityLow {
		t.Fatalf("preload priority = %d, want %d", got, resourcePriorityLow)
	}
	if got := backgroundResourcePriority(target, priorities); got != resourcePriorityLow {
		t.Fatalf("background preload priority = %d, want %d", got, resourcePriorityLow)
	}
	if got := backgroundResourcePriority("https://example.com/default.png", priorities); got != resourcePriorityHigh {
		t.Fatalf("default background priority = %d, want %d", got, resourcePriorityHigh)
	}
}

func TestVisibleImageWorkStartsAheadOfLazyBacklog(t *testing.T) {
	document := dom.NewDocument()
	visible := document.CreateElement("img", map[string]string{"src": "/visible.png"})
	lazy := document.CreateElement("img", map[string]string{"src": "/lazy.png", "loading": "lazy"})
	baseURL := mustParseURL(t, "https://example.com/")
	visiblePriority := imageResourcePriority(visible, resolveImageCandidate(baseURL, "/visible.png"), true, nil)
	lazyPriority := imageResourcePriority(lazy, resolveImageCandidate(baseURL, "/lazy.png"), false, nil)
	if visiblePriority <= lazyPriority {
		t.Fatalf("visible/lazy priorities = %d/%d", visiblePriority, lazyPriority)
	}

	release := make(chan struct{})
	started := make(chan string, maxResourceWorkers)
	jobs := make([]resourceJob, 0, maxResourceQueue)
	for index := 0; index < maxResourceQueue-1; index++ {
		jobs = append(jobs, resourceJob{priority: lazyPriority, order: index, run: func(context.Context) {
			select {
			case started <- "lazy":
			default:
			}
			<-release
		}})
	}
	jobs = append(jobs, resourceJob{priority: visiblePriority, order: maxResourceQueue - 1, run: func(context.Context) {
		select {
		case started <- "visible":
		default:
		}
		<-release
	}})
	done := make(chan struct{})
	go func() {
		runBoundedResourceJobs(context.Background(), jobs)
		close(done)
	}()
	first := make([]string, 0, maxResourceWorkers)
	for range maxResourceWorkers {
		select {
		case name := <-started:
			first = append(first, name)
		case <-time.After(time.Second):
			t.Fatal("resource workers did not start")
		}
	}
	close(release)
	<-done
	for _, name := range first {
		if name == "visible" {
			return
		}
	}
	t.Fatalf("first resource jobs = %v, want visible image", first)
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
