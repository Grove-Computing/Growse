package main

import (
	"io"
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
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"href=\"#resource\"", "id=\"resource\"", "tabindex=\"-1\""} {
		if !strings.Contains(string(body), marker) {
			t.Fatalf("showcase markup is missing %q", marker)
		}
	}
}
