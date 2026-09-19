package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
)

//go:embed index.html style.css app.mjs late.svg
var assets embed.FS

func handler() http.Handler { return http.FileServer(http.FS(assets)) }

func main() {
	address := "localhost:6053"
	flag.StringVar(&address, "addr", address, "Visual Fidelity showcase listen address")
	flag.Parse()
	fmt.Printf("Growse Visual Fidelity Showcase: http://%s\n", address)
	log.Fatal(http.ListenAndServe(address, handler()))
}
