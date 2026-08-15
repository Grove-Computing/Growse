package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
)

//go:embed *.html *.css _*.go
var workspaceAssets embed.FS

func workspaceHandler() http.Handler {
	mux := http.NewServeMux()
	assets, err := fs.Sub(workspaceAssets, ".")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	mux.HandleFunc("/login", func(response http.ResponseWriter, _ *http.Request) {
		http.SetCookie(response, &http.Cookie{Name: "workspace_session", Value: "local-demo", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		response.Header().Set("Content-Type", "text/plain")
		_, _ = response.Write([]byte("signed-in"))
	})
	mux.HandleFunc("/api/activity", func(response http.ResponseWriter, request *http.Request) {
		if _, err := request.Cookie("workspace_session"); err != nil {
			http.Error(response, "signed-out", http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "text/plain")
		response.Header().Set("Cache-Control", "private, max-age=30")
		_, _ = response.Write([]byte("workspace profile synchronized"))
	})
	mux.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" {
			http.Redirect(response, request, "/notes.html", http.StatusFound)
			return
		}
		response.Header().Set("Cache-Control", "max-age=3600")
		files.ServeHTTP(response, request)
	})
	return mux
}

func main() {
	address := "localhost:8080"
	fmt.Printf("Multi-Tab Workspace: http://%s\n", address)
	log.Fatal(http.ListenAndServe(address, workspaceHandler()))
}
