package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
)

//go:embed index.html style.css _app.go app.js
var dualRuntimeAssets embed.FS

func dualRuntimeHandler() http.Handler {
	mux := http.NewServeMux()
	assets, err := fs.Sub(dualRuntimeAssets, ".")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	mux.HandleFunc("/api/message", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/plain")
		_, _ = response.Write([]byte("offline fixture ready"))
	})
	mux.HandleFunc("/api/failure", func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "intentional fixture failure", http.StatusServiceUnavailable)
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
	address := "localhost:6053"
	flag.StringVar(&address, "addr", address, "showcase listen address")
	flag.Parse()
	fmt.Printf("Growse Dual Runtime Showcase: http://%s\n", address)
	log.Fatal(http.ListenAndServe(address, dualRuntimeHandler()))
}
