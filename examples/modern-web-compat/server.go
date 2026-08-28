package main

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"log"
	"net/http"

	woff2 "github.com/pgaskin/go-woff2"
	"golang.org/x/image/font/gofont/goregular"
)

//go:embed index.html style.css fixtures/nextjs/* fixtures/sveltekit/* fixtures/tailwind/* fixtures/diagnostics/*
var modernWebCompatibilityAssets embed.FS

var (
	showcaseFont = mustShowcaseFont()
	showcasePNG  = mustShowcasePNG()
)

func modernWebCompatibilityHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/next/", nextJSFixtureHandler())
	mux.Handle("/_next/static/", nextJSFixtureHandler())
	mux.Handle("/svelte/", svelteKitFixtureHandler())
	mux.Handle("/_app/immutable/", svelteKitFixtureHandler())
	mux.Handle("/tailwind/", tailwindFixtureHandler())
	mux.Handle("/diagnostics/", diagnosticsFixtureHandler())
	mux.HandleFunc("/assets/growse-regular.woff2", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "font/woff2")
		_, _ = response.Write(showcaseFont)
	})
	mux.HandleFunc("/assets/pixel.png", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "image/png")
		_, _ = response.Write(showcasePNG)
	})
	mux.Handle("/", http.FileServer(http.FS(modernWebCompatibilityAssets)))
	return mux
}

func nextJSFixtureHandler() http.Handler {
	assets, err := fs.Sub(modernWebCompatibilityAssets, "fixtures/nextjs")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/next/", "/next/about":
			request.URL.Path = "/"
		case "/_next/static/css/app.css":
			request.URL.Path = "/app.css"
		case "/_next/static/chunks/app.mjs":
			request.URL.Path = "/app.mjs"
		case "/_next/static/chunks/counter.chunk.mjs":
			request.URL.Path = "/counter.chunk.mjs"
		default:
			http.NotFound(response, request)
			return
		}
		files.ServeHTTP(response, request)
	})
}

func svelteKitFixtureHandler() http.Handler {
	assets, err := fs.Sub(modernWebCompatibilityAssets, "fixtures/sveltekit")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/svelte/", "/svelte/about":
			request.URL.Path = "/"
		case "/_app/immutable/assets/app.css":
			request.URL.Path = "/app.css"
		case "/_app/immutable/entry/start.mjs":
			request.URL.Path = "/start.mjs"
		case "/_app/immutable/nodes/app.mjs":
			request.URL.Path = "/app.mjs"
		default:
			http.NotFound(response, request)
			return
		}
		files.ServeHTTP(response, request)
	})
}

func tailwindFixtureHandler() http.Handler {
	assets, err := fs.Sub(modernWebCompatibilityAssets, "fixtures/tailwind")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/tailwind/":
			request.URL.Path = "/"
		case "/tailwind/app.css":
			request.URL.Path = "/app.css"
		default:
			http.NotFound(response, request)
			return
		}
		files.ServeHTTP(response, request)
	})
}

func diagnosticsFixtureHandler() http.Handler {
	assets, err := fs.Sub(modernWebCompatibilityAssets, "fixtures/diagnostics")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/diagnostics/":
			request.URL.Path = "/"
		case "/diagnostics/failures.mjs":
			request.URL.Path = "/failures.mjs"
		case "/diagnostics/bad.woff2":
			response.Header().Set("Content-Type", "font/woff2")
			_, _ = response.Write([]byte("malformed local font"))
			return
		case "/diagnostics/bad.png":
			response.Header().Set("Content-Type", "image/png")
			_, _ = response.Write([]byte("malformed local image"))
			return
		default:
			http.NotFound(response, request)
			return
		}
		files.ServeHTTP(response, request)
	})
}

func mustShowcaseFont() []byte {
	encoded, err := woff2.Encode(goregular.TTF, nil)
	if err != nil {
		panic(err)
	}
	return encoded
}

func mustShowcasePNG() []byte {
	pixel := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			pixel.Set(x, y, color.NRGBA{R: uint8(40 + x*4), G: uint8(80 + y*3), B: 200, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixel); err != nil {
		panic(err)
	}
	return encoded.Bytes()
}

func main() {
	address := "localhost:8080"
	fmt.Printf("Growse Modern Web Compatibility Showcase: http://%s\n", address)
	log.Fatal(http.ListenAndServe(address, modernWebCompatibilityHandler()))
}
