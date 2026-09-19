package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVisualFidelityShowcaseServesInteractiveSurface(t *testing.T) {
	server := httptest.NewServer(handler())
	defer server.Close()
	for _, path := range []string{"/", "/style.css", "/app.mjs", "/late.svg"} {
		response, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("GET %s = %d", path, response.StatusCode)
		}
	}
	response, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if !strings.Contains(response.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("content type = %q", response.Header.Get("Content-Type"))
	}
}
