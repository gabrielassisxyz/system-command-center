package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// ErrProcessNotFound is returned by a ProcessKiller when the target process
// does not exist. The HTTP handler maps it to a 404 status.
var ErrProcessNotFound = errors.New("process not found")

// ProcessKiller abstracts sending a signal to a process by PID. The
// production implementation sends SIGTERM; tests inject a fake to verify
// the HTTP endpoint parses and invokes it correctly.
type ProcessKiller interface {
	Kill(pid int) error
}

// killActionHandler handles POST /api/process/kill?pid=<n>.
// It parses the pid, delegates to the injected killer, and maps errors to
// HTTP status codes without panicking.
func killActionHandler(killer ProcessKiller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		pidStr := r.URL.Query().Get("pid")
		if pidStr == "" {
			http.Error(w, "missing pid", http.StatusBadRequest)
			return
		}

		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid <= 0 {
			http.Error(w, "invalid pid", http.StatusBadRequest)
			return
		}

		if err := killer.Kill(pid); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, ErrProcessNotFound) {
				status = http.StatusNotFound
			}
			http.Error(w, fmt.Sprintf("kill failed: %v", err), status)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
