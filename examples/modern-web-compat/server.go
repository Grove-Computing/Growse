package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed fixtures/nextjs/*
var modernWebCompatibilityAssets embed.FS

func modernWebCompatibilityHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/next/", nextJSFixtureHandler())
	mux.Handle("/_next/static/", nextJSFixtureHandler())
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
