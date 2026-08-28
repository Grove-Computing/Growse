package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed fixtures/nextjs/* fixtures/sveltekit/* fixtures/tailwind/*
var modernWebCompatibilityAssets embed.FS

func modernWebCompatibilityHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/next/", nextJSFixtureHandler())
	mux.Handle("/_next/static/", nextJSFixtureHandler())
	mux.Handle("/svelte/", svelteKitFixtureHandler())
	mux.Handle("/_app/immutable/", svelteKitFixtureHandler())
	mux.Handle("/tailwind/", tailwindFixtureHandler())
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
