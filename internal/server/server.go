// Package server wires the HTTP surface of hardware-usage.
package server

import (
	"context"
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
// It serves an embedded static page at "/", static assets under "/",
// live snapshots at "/events", and the process/container action endpoints.
func NewMux(provider SnapshotProvider) *http.ServeMux {
	hub := NewSSEHub(provider, 2*time.Second)
	go hub.Run()
	controller, err := NewDockerController()
	if err != nil {
		// The controller only fails on an unsupported DOCKER_HOST value,
		// which is a configuration error at startup.
		panic(err)
	}
	return newMux(hub, OsProcessKiller{}, controller)
}

// NewMuxWithHub returns a mux using an already-created SSEHub.
// Useful in tests to avoid starting the real ticker.
func NewMuxWithHub(hub *SSEHub) *http.ServeMux {
	return newMux(hub, nilKiller{}, nilController{})
}

func newMux(hub *SSEHub, killer ProcessKiller, controller DockerController) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/events", hub)
	mux.HandleFunc("/api/process/kill", killActionHandler(killer))
	mux.HandleFunc("/api/container/stop", containerActionHandler(controller, controller.Stop))
	mux.HandleFunc("/api/container/restart", containerActionHandler(controller, controller.Restart))
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

// nilKiller is a no-op ProcessKiller used when wiring tests that never hit
// the action endpoints.
type nilKiller struct{}

func (nilKiller) Kill(int) error { return nil }

// nilController is a no-op DockerController used when wiring tests that never
// hit the container action endpoints.
type nilController struct{}

func (nilController) Stop(_ context.Context, _ string) error    { return nil }
func (nilController) Restart(_ context.Context, _ string) error { return nil }
