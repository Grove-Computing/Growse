package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
)

//go:embed index.html style.css classic.js app.mjs dependency.mjs dynamic.mjs same-frame.html sandbox-frame.html sw.js
var externalWebPlatformAssets embed.FS

//go:embed cross-frame.html
var frameAssets embed.FS

func externalWebPlatformHandler(cdnOrigin, frameOrigin string) http.Handler {
	mux := http.NewServeMux()
	assets, err := fs.Sub(externalWebPlatformAssets, ".")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	mux.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" || request.URL.Path == "/app/" {
			content, readErr := externalWebPlatformAssets.ReadFile("index.html")
			if readErr != nil {
				http.Error(response, readErr.Error(), http.StatusInternalServerError)
				return
			}
			page := strings.ReplaceAll(string(content), "{{CDN_ORIGIN}}", cdnOrigin)
			page = strings.ReplaceAll(page, "{{FRAME_ORIGIN}}", frameOrigin)
			response.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = response.Write([]byte(page))
			return
		}
		if request.URL.Path == "/answer.wasm" {
			response.Header().Set("Content-Type", "application/wasm")
			_, _ = response.Write(answerWASM)
			return
		}
		if request.URL.Path == "/app/sw.js" {
			content, readErr := externalWebPlatformAssets.ReadFile("sw.js")
			if readErr != nil {
				http.Error(response, readErr.Error(), http.StatusInternalServerError)
				return
			}
			response.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = response.Write(content)
			return
		}
		files.ServeHTTP(response, request)
	})
	return mux
}

func externalCDNHandler() http.Handler {
	mux := http.NewServeMux()
	assets, err := fs.Sub(externalWebPlatformAssets, ".")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	mux.HandleFunc("/external.css", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = response.Write([]byte(`@import "/theme.css"; .cdn-style { color: #0b7285; }`))
	})
	mux.HandleFunc("/theme.css", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = response.Write([]byte(`.cdn-style { border-color: #15aabf; }`))
	})
	mux.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Access-Control-Allow-Origin", "*")
		if strings.HasSuffix(request.URL.Path, ".mjs") {
			response.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		}
		files.ServeHTTP(response, request)
	})
	return mux
}

func externalFrameHandler() http.Handler {
	return http.FileServer(http.FS(frameAssets))
}

var answerWASM = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7f,
	0x03, 0x02, 0x01, 0x00,
	0x07, 0x0a, 0x01, 0x06, 0x61, 0x6e, 0x73, 0x77, 0x65, 0x72, 0x00, 0x00,
	0x0a, 0x06, 0x01, 0x04, 0x00, 0x41, 0x2a, 0x0b,
}

func main() {
	go func() { log.Fatal(http.ListenAndServe("localhost:8081", externalCDNHandler())) }()
	go func() { log.Fatal(http.ListenAndServe("localhost:8082", externalFrameHandler())) }()
	address := "localhost:8080"
	fmt.Printf("Growse External Web Platform Showcase: http://%s/app/\n", address)
	log.Fatal(http.ListenAndServe(address, externalWebPlatformHandler("http://localhost:8081", "http://localhost:8082")))
}
