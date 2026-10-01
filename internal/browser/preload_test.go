package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
)

func TestHTMLPreloadScannerStartsStylesheetBeforeDocumentBodyCompletes(t *testing.T) {
	pageRelease := make(chan struct{})
	styleRelease := make(chan struct{})
	styleStarted := make(chan struct{}, 1)
	var styleRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/page":
			response.Header().Set("Content-Type", "text/html")
			_, _ = response.Write([]byte(`<link rel="stylesheet" href="/style.css"><main`))
			response.(http.Flusher).Flush()
			<-pageRelease
			_, _ = response.Write([]byte(` id="ready">ready</main>`))
		case "/style.css":
			styleRequests.Add(1)
			select {
			case styleStarted <- struct{}{}:
			default:
			}
			<-styleRelease
			response.Header().Set("Content-Type", "text/css")
			_, _ = response.Write([]byte(`main { color: red; }`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	browserState := New(network.NewClientWithLimits(server.Client(), 1<<20))
	done := make(chan error, 1)
	go func() { _, err := browserState.Navigate(context.Background(), server.URL+"/page"); done <- err }()
	select {
	case <-styleStarted:
	case <-time.After(time.Second):
		close(pageRelease)
		close(styleRelease)
		t.Fatal("stylesheet did not start while the HTML body was streaming")
	}
	close(pageRelease)
	close(styleRelease)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("navigation did not finish")
	}
	if got := styleRequests.Load(); got != 1 {
		t.Fatalf("stylesheet requests = %d, want one coalesced request", got)
	}
}
