// Package server wires the HTTP surface of hardware-usage.
package server

import "net/http"

// NewMux returns the HTTP handler for the live-usage web page. v1 is a placeholder;
// the per-process and compose-stack views land via the task_plan.md loop.
func NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}
