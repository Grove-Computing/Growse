package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"time"
)

//go:embed index.html style.css app.mjs
var cssLayoutAssets embed.FS

var lateLayoutImage = makeLateLayoutImage()

func cssLayoutHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/app.mjs", func(response http.ResponseWriter, _ *http.Request) {
		module, err := cssLayoutAssets.ReadFile("app.mjs")
		if err != nil {
			http.Error(response, "showcase module is unavailable", http.StatusInternalServerError)
			return
		}
		response.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = response.Write(module)
	})
	mux.HandleFunc("/assets/late-layout.png", func(response http.ResponseWriter, _ *http.Request) {
		time.Sleep(180 * time.Millisecond)
		response.Header().Set("Content-Type", "image/png")
		_, _ = response.Write(lateLayoutImage)
	})
	mux.Handle("/", http.FileServer(http.FS(cssLayoutAssets)))
	return mux
}

func makeLateLayoutImage() []byte {
	canvas := image.NewNRGBA(image.Rect(0, 0, 240, 120))
	for y := range 120 {
		for x := range 240 {
			canvas.Set(x, y, color.NRGBA{R: uint8(35 + x/3), G: uint8(70 + y), B: 190, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		panic(err)
	}
	return encoded.Bytes()
}

func main() {
	address := "localhost:18081"
	flag.StringVar(&address, "addr", address, "CSS Layout 2026 showcase listen address")
	flag.Parse()
	fmt.Printf("Growse CSS Layout 2026 Showcase: http://%s\n", address)
	log.Fatal(http.ListenAndServe(address, cssLayoutHandler()))
}
