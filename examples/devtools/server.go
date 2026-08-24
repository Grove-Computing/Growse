package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"
)

//go:embed index.html style.css _app.go
var devToolsAssets embed.FS

func devToolsHandler() http.Handler {
	mux := http.NewServeMux()
	assets, err := fs.Sub(devToolsAssets, ".")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	mux.HandleFunc("/api/success", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/plain")
		_, _ = response.Write([]byte("success"))
	})
	mux.HandleFunc("/api/redirect", func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, "/api/success", http.StatusFound)
	})
	mux.HandleFunc("/api/cache", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Cache-Control", "max-age=60")
		response.Header().Set("Content-Type", "text/plain")
		_, _ = response.Write([]byte("cacheable"))
	})
	mux.HandleFunc("/api/error", func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "fixture error", http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/api/slow", func(response http.ResponseWriter, request *http.Request) {
		select {
		case <-time.After(250 * time.Millisecond):
			_, _ = response.Write([]byte("late"))
		case <-request.Context().Done():
		}
	})
	mux.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" {
			response.Header().Set("Content-Type", "text/html")
		}
		files.ServeHTTP(response, request)
	})
	return mux
}

func main() {
	address := "localhost:8080"
	fmt.Printf("Growse DevTools Showcase: http://%s\n", address)
	log.Fatal(http.ListenAndServe(address, devToolsHandler()))
}
