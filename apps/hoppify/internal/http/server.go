package httpapi

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"time"
)

//go:embed web/index.html web/assets/*
var webFiles embed.FS

type liveResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func NewHandler(startedAt time.Time) http.Handler {
	mux := http.NewServeMux()
	webRoot := mustSub(webFiles, "web")

	mux.Handle("GET /assets/", http.FileServer(http.FS(webRoot)))
	mux.HandleFunc("GET /", serveIndex(webRoot))
	mux.HandleFunc("GET /live", serveLive)
	mux.HandleFunc("GET /metrics", serveMetrics(startedAt))

	return mux
}

func serveIndex(webRoot fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFileFS(w, r, webRoot, "index.html")
	}
}

func serveLive(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(liveResponse{Status: "ok", Service: "hoppify"}); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
}

func serveMetrics(startedAt time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		uptime := time.Since(startedAt).Seconds()
		metrics := fmt.Sprintf(`# HELP hoppify_up Whether the Hoppify process is running.
# TYPE hoppify_up gauge
hoppify_up 1
# HELP hoppify_uptime_seconds Hoppify process uptime in seconds.
# TYPE hoppify_uptime_seconds gauge
hoppify_uptime_seconds %.0f
`, uptime)
		_, _ = w.Write([]byte(metrics))
	}
}

func mustSub(files fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(files, dir)
	if err != nil {
		panic(err)
	}

	return sub
}
