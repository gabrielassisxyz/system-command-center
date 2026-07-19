package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type recordingKiller struct {
	mu   sync.Mutex
	pids []int
	err  error
}

func (k *recordingKiller) Kill(pid int) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.pids = append(k.pids, pid)
	return k.err
}

func (k *recordingKiller) Pids() []int {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]int, len(k.pids))
	copy(out, k.pids)
	return out
}

func TestKillEndpointInvokesKiller(t *testing.T) {
	killer := &recordingKiller{}
	req := httptest.NewRequest(http.MethodPost, "/api/process/kill?pid=42", nil)
	rec := httptest.NewRecorder()

	killActionHandler(killer).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	pids := killer.Pids()
	if len(pids) != 1 || pids[0] != 42 {
		t.Errorf("killed pids = %v, want [42]", pids)
	}
}

func TestKillEndpointMissingPid(t *testing.T) {
	killer := &recordingKiller{}
	req := httptest.NewRequest(http.MethodPost, "/api/process/kill", nil)
	rec := httptest.NewRecorder()

	killActionHandler(killer).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "missing pid") {
		t.Errorf("body = %q, want missing pid message", rec.Body.String())
	}
	if len(killer.Pids()) != 0 {
		t.Errorf("killer called unexpectedly: %v", killer.Pids())
	}
}

func TestKillEndpointBadPid(t *testing.T) {
	killer := &recordingKiller{}
	for _, qs := range []string{"?pid=abc", "?pid=0", "?pid=-5"} {
		req := httptest.NewRequest(http.MethodPost, "/api/process/kill"+qs, nil)
		rec := httptest.NewRecorder()

		killActionHandler(killer).ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want %d", qs, rec.Code, http.StatusBadRequest)
		}
		if !strings.Contains(rec.Body.String(), "invalid pid") {
			t.Errorf("%s body = %q, want invalid pid message", qs, rec.Body.String())
		}
	}
	if len(killer.Pids()) != 0 {
		t.Errorf("killer called unexpectedly: %v", killer.Pids())
	}
}

func TestKillEndpointEmptyPid(t *testing.T) {
	killer := &recordingKiller{}
	req := httptest.NewRequest(http.MethodPost, "/api/process/kill?pid=", nil)
	rec := httptest.NewRecorder()

	killActionHandler(killer).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "missing pid") {
		t.Errorf("body = %q, want missing pid message", rec.Body.String())
	}
	if len(killer.Pids()) != 0 {
		t.Errorf("killer called unexpectedly: %v", killer.Pids())
	}
}

func TestKillEndpointKillerError(t *testing.T) {
	killer := &recordingKiller{err: errors.New("permission denied")}
	req := httptest.NewRequest(http.MethodPost, "/api/process/kill?pid=1", nil)
	rec := httptest.NewRecorder()

	killActionHandler(killer).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "kill failed") || !strings.Contains(body, "permission denied") {
		t.Errorf("body = %q, want kill failed message", body)
	}
}

func TestKillEndpointProcessNotFound(t *testing.T) {
	killer := &recordingKiller{err: ErrProcessNotFound}
	req := httptest.NewRequest(http.MethodPost, "/api/process/kill?pid=999999", nil)
	rec := httptest.NewRecorder()

	killActionHandler(killer).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestKillEndpointWrongMethod(t *testing.T) {
	killer := &recordingKiller{}
	req := httptest.NewRequest(http.MethodGet, "/api/process/kill?pid=42", nil)
	rec := httptest.NewRecorder()

	killActionHandler(killer).ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
