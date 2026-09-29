package javascript

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	fetchapi "github.com/Grove-Computing/Growse/internal/webapi/fetch"
)

func TestXMLHttpRequestLoadsJSONAndDispatchesLifecycleEvents(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page")
	requestSeen := make(chan *network.Request, 1)
	messages := make(chan string, 1)
	environment := runtimemodel.Environment{
		BaseURL:      baseURL,
		FetchLimiter: fetchapi.NewLimiter(8),
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			requestSeen <- request
			return &network.Response{
				URL: request.URL, StatusCode: http.StatusOK, Status: "OK",
				Header: http.Header{"Content-Type": {"application/json"}, "X-Result": {"yes"}},
				Body:   []byte(`{"name":"Growse"}`),
			}, nil
		},
		ConsoleRecord: func(_, message string) { messages <- message },
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		var events = [];
		var xhr = new XMLHttpRequest();
		xhr.addEventListener("loadstart", function () { events.push("loadstart"); });
		xhr.onreadystatechange = function () { events.push("state:" + xhr.readyState); };
		xhr.onload = function () {
			events.push("load");
			console.log([
				xhr.status,
				xhr.statusText,
				xhr.responseURL,
				xhr.getResponseHeader("X-Result"),
				xhr.response.name,
				events.join(",")
			].join("|"));
		};
		xhr.open("GET", "/api");
		xhr.setRequestHeader("X-Test", "xhr");
		xhr.responseType = "json";
		xhr.send();`
	startJavaScriptRuntime(t, runtime, source, environment)
	request := receiveRequest(t, requestSeen)
	if request.Method != http.MethodGet || request.URL.String() != "https://example.test/api" || request.Header.Get("X-Test") != "xhr" {
		t.Fatalf("XMLHttpRequest request = %#v", request)
	}
	want := "200|OK|https://example.test/api|yes|Growse|state:1,loadstart,state:2,state:3,state:4,load"
	if got := receiveMessage(t, messages); got != want {
		t.Fatalf("XMLHttpRequest result = %q, want %q", got, want)
	}
}
