// Package server wires the HTTP surface of hardware-usage.
package server

import (
	"embed"
	"io/fs"
	"net/http"
	"time"
)

//go:embed static
var staticFS embed.FS

// DefaultAddr returns the default loopback listen address used by this server.
func DefaultAddr() string {
	return "127.0.0.1:8765"
}

// NewMux returns the HTTP handler for the live-usage web page.
// It serves an embedded static page at "/", static assets under "/", and
// live snapshots at "/events".
func NewMux(provider SnapshotProvider) *http.ServeMux {
	hub := NewSSEHub(provider, 2*time.Second)
	go hub.Run()
	return newMux(hub)
}

// NewMuxWithHub returns a mux using an already-created SSEHub.
// Useful in tests to avoid starting the real ticker.
func NewMuxWithHub(hub *SSEHub) *http.ServeMux {
	return newMux(hub)
}

func newMux(hub *SSEHub) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/events", hub)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	public, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(public)))
	return mux
}
