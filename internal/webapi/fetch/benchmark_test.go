package fetch

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
)

func BenchmarkComplete16ConcurrentFetches(b *testing.B) {
	baseURL, err := url.Parse("https://benchmark.example.test/page")
	if err != nil {
		b.Fatal(err)
	}
	queue := make(chan func(), 32)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for callback := range queue {
			callback()
		}
	}()
	api := NewPage(context.Background(), baseURL, func(_ context.Context, request *network.Request) (*network.Response, error) {
		return &network.Response{URL: request.URL, StatusCode: http.StatusOK}, nil
	}, func(callback func()) bool {
		queue <- callback
		return true
	})
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		var completed sync.WaitGroup
		completed.Add(16)
		for requestIndex := 0; requestIndex < 16; requestIndex++ {
			api.Fetch(Request{URL: "/data"}, func(Response) { completed.Done() }, func(string) { completed.Done() })
		}
		completed.Wait()
	}
	b.StopTimer()
	close(queue)
	<-workerDone
}
