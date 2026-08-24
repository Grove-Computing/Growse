package browser

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

type devToolsRoundTripFunc func(*http.Request) (*http.Response, error)

func (function devToolsRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestDevToolsNetworkRecordsNavigationResourcesFetchAndFormByPage(t *testing.T) {
	var pixel bytes.Buffer
	pixelImage := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	pixelImage.Set(0, 0, color.White)
	if err := png.Encode(&pixel, pixelImage); err != nil {
		t.Fatal(err)
	}
	client := network.NewClientWithLimits(&http.Client{Transport: devToolsRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		contentType := "text/html"
		body := `<link rel="stylesheet" href="/style.css"><style>body { background-image: url('/pixel.png') }</style>
<script type="text/go" src="/app.go"></script><form id="form" method="post" action="/submit"><input name="value" value="safe"></form>`
		switch request.URL.Path {
		case "/style.css":
			contentType, body = "text/css", "body { color: blue }"
		case "/pixel.png":
			contentType, body = "image/png", pixel.String()
		case "/app.go":
			contentType, body = "text/x-go", "package main; func main() {}"
		case "/data":
			contentType, body = "text/plain", "ok"
		case "/submit":
			body = "<p>submitted</p>"
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}, 1<<20)
	runtime := &runtimeStub{}
	browserState := NewWithRuntimeFactory(client, func() runtimemodel.Runtime { return runtime })

	page, err := browserState.Navigate(context.Background(), "http://user:password@localhost/index?token=secret")
	if err != nil {
		t.Fatal(err)
	}
	dataURL := mustParseURL(t, "http://localhost/data?api_key=secret")
	if _, err := runtime.environment.Fetch(context.Background(), &network.Request{Method: http.MethodGet, URL: dataURL, SiteURL: page.URL, Kind: network.RequestFetch, Header: http.Header{"Authorization": []string{"Bearer secret"}, "Cookie": []string{"session=secret"}}, Body: []byte("must not be retained")}); err != nil {
		t.Fatal(err)
	}
	records := page.DevTools.Network()
	for _, kind := range []string{"navigation", "stylesheet", "image", "script", "fetch"} {
		if !hasNetworkKind(records, kind) {
			t.Fatalf("missing %s record: %+v", kind, records)
		}
	}
	for _, record := range records {
		if strings.Contains(record.URL, "password") || strings.Contains(record.URL, "secret") || strings.Contains(record.URL, "Bearer") || strings.Contains(record.URL, "session") {
			t.Fatalf("credential leaked into record: %+v", record)
		}
	}

	form, ok := page.Document.GetElementByID("form")
	if !ok {
		t.Fatal("form not found")
	}
	submitted, err := browserState.SubmitPOST(context.Background(), form.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	formRecords := submitted.DevTools.Network()
	if len(formRecords) == 0 || formRecords[0].Kind != "form" || formRecords[0].Method != http.MethodPost {
		t.Fatalf("form records = %+v", formRecords)
	}
	if got := len(page.DevTools.Network()); got != 0 {
		t.Fatalf("retired page retained %d records", got)
	}
}

func hasNetworkKind(records []devtools.NetworkRecord, kind string) bool {
	for _, record := range records {
		if record.Kind == kind {
			return true
		}
	}
	return false
}
