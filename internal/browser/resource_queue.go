package browser

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/Grove-Computing/Growse/internal/dom"
)

const (
	maxResourceWorkers = 4
	maxResourceQueue   = 256
)

type resourcePriority uint8

const (
	resourcePriorityLow resourcePriority = iota
	resourcePriorityNormal
	resourcePriorityHigh
	resourcePriorityCritical
)

type resourceJob struct {
	priority resourcePriority
	order    int
	run      func(context.Context)
}

// runBoundedResourceJobs runs a stable priority queue with a fixed worker and
// pending-work budget. The return value is the number of jobs rejected before
// execution, allowing the caller to localize queue saturation per resource.
func runBoundedResourceJobs(ctx context.Context, jobs []resourceJob) int {
	if ctx == nil {
		ctx = context.Background()
	}
	sort.SliceStable(jobs, func(left, right int) bool {
		if jobs[left].priority == jobs[right].priority {
			return jobs[left].order < jobs[right].order
		}
		return jobs[left].priority > jobs[right].priority
	})
	rejected := 0
	if len(jobs) > maxResourceQueue {
		rejected = len(jobs) - maxResourceQueue
		jobs = jobs[:maxResourceQueue]
	}
	workers := min(maxResourceWorkers, len(jobs))
	if workers == 0 {
		return rejected
	}
	pending := make(chan resourceJob, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for job := range pending {
				if ctx.Err() == nil && job.run != nil {
					job.run(ctx)
				}
			}
		}()
	}
	for _, job := range jobs {
		select {
		case pending <- job:
		case <-ctx.Done():
			close(pending)
			group.Wait()
			return rejected
		}
	}
	close(pending)
	group.Wait()
	return rejected
}

func imageResourcePriority(node *dom.Node, target *url.URL, eligible bool, preloads map[string]bool) resourcePriority {
	if node == nil {
		return resourcePriorityLow
	}
	if value, _ := node.Attribute("fetchpriority"); strings.EqualFold(strings.TrimSpace(value), "high") {
		return resourcePriorityCritical
	} else if strings.EqualFold(strings.TrimSpace(value), "low") {
		return resourcePriorityLow
	}
	if target != nil && preloads[target.String()] {
		return resourcePriorityCritical
	}
	if loading, _ := node.Attribute("loading"); strings.EqualFold(strings.TrimSpace(loading), "lazy") && !eligible {
		return resourcePriorityLow
	}
	if eligible {
		return resourcePriorityHigh
	}
	return resourcePriorityNormal
}

func imagePreloads(document *dom.Document, baseURL *url.URL) map[string]bool {
	preloads := make(map[string]bool)
	if document == nil || baseURL == nil {
		return preloads
	}
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "link" {
			rel, _ := node.Attribute("rel")
			as, _ := node.Attribute("as")
			href, _ := node.Attribute("href")
			if containsSpaceToken(rel, "preload") && strings.EqualFold(strings.TrimSpace(as), "image") {
				if target := resolveImageCandidate(baseURL, href); target != nil {
					preloads[target.String()] = true
				}
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(document.Root)
	return preloads
}

func containsSpaceToken(value, token string) bool {
	for _, field := range strings.Fields(value) {
		if strings.EqualFold(field, token) {
			return true
		}
	}
	return false
}
