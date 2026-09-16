package browser

import (
	"sync"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestRecomputePageStylesSerializesConcurrentRuntimeMutations(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	html := document.CreateElement("html", nil)
	body := document.CreateElement("body", nil)
	if err := document.AppendChild(document.Root, html); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(html, body); err != nil {
		t.Fatal(err)
	}
	page := &Page{
		Document:       document,
		ComputedStyles: style.Compute(document, nil),
		Animations:     style.NewAnimationRegistry(),
		Transitions:    style.NewTransitionRegistry(),
		ViewportWidth:  800,
		ViewportHeight: 600,
	}

	const workerCount = 8
	const iterations = 50
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func(offset int) {
			defer workers.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				recomputePageStyles(page, time.Unix(int64(offset), int64(iteration)))
			}
		}(worker)
	}
	workers.Wait()

	if got, want := page.StyleRevision, uint64(workerCount*iterations); got != want {
		t.Fatalf("style revision = %d, want %d", got, want)
	}
}
