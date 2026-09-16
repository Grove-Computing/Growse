package main

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCSSLayoutShowcaseServesSizingTableAndLateImage(t *testing.T) {
	server := httptest.NewServer(cssLayoutHandler())
	defer server.Close()
	for _, route := range []struct {
		path, contentType, marker string
	}{
		{"/", "text/html", "CSS Layout 2026"},
		{"/style.css", "text/css", "table-layout: fixed"},
		{"/app.mjs", "text/javascript", "COLLAPSED · AUTO"},
		{"/assets/late-layout.png", "image/png", ""},
	} {
		response, err := server.Client().Get(server.URL + route.path)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != 200 || !strings.Contains(response.Header.Get("Content-Type"), route.contentType) ||
			route.marker != "" && !strings.Contains(string(body), route.marker) {
			t.Fatalf("GET %s = status:%d type:%q marker:%t err:%v", route.path, response.StatusCode, response.Header.Get("Content-Type"), strings.Contains(string(body), route.marker), readErr)
		}
	}
}
