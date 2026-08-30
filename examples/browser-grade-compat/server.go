package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	woff2 "github.com/pgaskin/go-woff2"
	"golang.org/x/image/font/gofont/goregular"
)

// browserGradeCompatibilityHandler serves the pinned framework artifacts from
// the sibling modern-web-compat fixture and overlays the v0.17.0 landing page.
func browserGradeCompatibilityHandler(root string) http.Handler {
	mux := http.NewServeMux()
	modernRoot := filepath.Join(root, "modern-web-compat")
	mux.HandleFunc("/showcase/", func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/showcase/" && request.URL.Path != "/showcase/index.html" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(writer, request, filepath.Join(root, "browser-grade-compat", "index.html"))
	})
	files := map[string]string{
		"/style.css":                                 "style.css",
		"/next/":                                     "fixtures/nextjs/index.html",
		"/next/about":                                "fixtures/nextjs/index.html",
		"/_next/static/css/app.css":                  "fixtures/nextjs/app.css",
		"/_next/static/chunks/app.mjs":               "fixtures/nextjs/app.mjs",
		"/_next/static/chunks/counter.chunk.mjs":     "fixtures/nextjs/counter.chunk.mjs",
		"/_next/static/chunks/upstream-contract.mjs": "fixtures/nextjs/upstream-contract.mjs",
		"/svelte/":                                   "fixtures/sveltekit/index.html",
		"/svelte/about":                              "fixtures/sveltekit/index.html",
		"/_app/immutable/assets/app.css":             "fixtures/sveltekit/app.css",
		"/_app/immutable/entry/start.mjs":            "fixtures/sveltekit/start.mjs",
		"/_app/immutable/nodes/app.mjs":              "fixtures/sveltekit/app.mjs",
		"/_app/immutable/upstream-contract.mjs":      "fixtures/sveltekit/upstream-contract.mjs",
		"/tailwind/":                                 "fixtures/tailwind/index.html",
		"/tailwind/app.css":                          "fixtures/tailwind/app.css",
		"/real-site/":                                "fixtures/real-site/index.html",
		"/real-site/app.css":                         "fixtures/real-site/app.css",
		"/real-site/app.mjs":                         "fixtures/real-site/app.mjs",
		"/diagnostics/":                              "fixtures/diagnostics/index.html",
		"/diagnostics/failures.mjs":                  "fixtures/diagnostics/failures.mjs",
	}
	mux.HandleFunc("/assets/growse-regular.woff2", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "font/woff2")
		encoded, err := woff2.Encode(goregular.TTF, nil)
		if err != nil {
			http.Error(writer, "font unavailable", http.StatusInternalServerError)
			return
		}
		_, _ = writer.Write(encoded)
	})
	mux.HandleFunc("/assets/pixel.png", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "image/png")
		_, _ = writer.Write(browserGradePNG())
	})
	mux.HandleFunc("/diagnostics/bad.woff2", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "font/woff2")
		_, _ = writer.Write([]byte("malformed local font"))
	})
	mux.HandleFunc("/diagnostics/bad.png", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "image/png")
		_, _ = writer.Write([]byte("malformed local image"))
	})
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		relative, exists := files[request.URL.Path]
		if !exists {
			http.NotFound(writer, request)
			return
		}
		switch filepath.Ext(relative) {
		case ".mjs", ".js":
			writer.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case ".css":
			writer.Header().Set("Content-Type", "text/css; charset=utf-8")
		}
		http.ServeFile(writer, request, filepath.Join(modernRoot, relative))
	})
	return mux
}

func browserGradePNG() []byte {
	pixel := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			pixel.Set(x, y, color.NRGBA{R: uint8(40 + x*3), G: uint8(110 + y*2), B: 220, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, pixel); err != nil {
		panic(err)
	}
	return buffer.Bytes()
}

func fixtureRoot() string {
	if value := os.Getenv("GROWSE_EXAMPLES_ROOT"); value != "" {
		return value
	}
	return ".."
}

func main() {
	_ = mime.AddExtensionType(".mjs", "text/javascript; charset=utf-8")
	address := "127.0.0.1:8091"
	fmt.Printf("Growse Browser-grade Compatibility Showcase: http://%s/showcase/\n", address)
	log.Fatal(http.ListenAndServe(address, browserGradeCompatibilityHandler(fixtureRoot())))
}
