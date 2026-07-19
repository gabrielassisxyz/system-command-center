package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type recordingController struct {
	mu         sync.Mutex
	stops      []string
	restarts   []string
	stopErr    error
	restartErr error
}

func (c *recordingController) Stop(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stops = append(c.stops, id)
	return c.stopErr
}

func (c *recordingController) Restart(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restarts = append(c.restarts, id)
	return c.restartErr
}

func (c *recordingController) Stops() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.stops))
	copy(out, c.stops)
	return out
}

func (c *recordingController) Restarts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.restarts))
	copy(out, c.restarts)
	return out
}

func TestStopEndpointInvokesController(t *testing.T) {
	ctl := &recordingController{}
	req := httptest.NewRequest(http.MethodPost, "/api/container/stop?id=abc123", nil)
	rec := httptest.NewRecorder()

	containerActionHandler(ctl, ctl.Stop).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	stops := ctl.Stops()
	if len(stops) != 1 || stops[0] != "abc123" {
		t.Errorf("stopped ids = %v, want [abc123]", stops)
	}
}

func TestRestartEndpointInvokesController(t *testing.T) {
	ctl := &recordingController{}
	req := httptest.NewRequest(http.MethodPost, "/api/container/restart?id=abc123", nil)
	rec := httptest.NewRecorder()

	containerActionHandler(ctl, ctl.Restart).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	restarts := ctl.Restarts()
	if len(restarts) != 1 || restarts[0] != "abc123" {
		t.Errorf("restarted ids = %v, want [abc123]", restarts)
	}
}

func TestContainerEndpointMissingID(t *testing.T) {
	ctl := &recordingController{}
	for _, path := range []string{"/api/container/stop", "/api/container/restart"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()

		handler := containerActionHandler(ctl, ctl.Stop)
		if strings.Contains(path, "restart") {
			handler = containerActionHandler(ctl, ctl.Restart)
		}
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusBadRequest)
		}
		if !strings.Contains(rec.Body.String(), "missing container id") {
			t.Errorf("%s body = %q, want missing container id message", path, rec.Body.String())
		}
	}
	if len(ctl.Stops()) != 0 || len(ctl.Restarts()) != 0 {
		t.Errorf("controller called unexpectedly: stops=%v restarts=%v", ctl.Stops(), ctl.Restarts())
	}
}

func TestContainerEndpointEmptyID(t *testing.T) {
	ctl := &recordingController{}
	req := httptest.NewRequest(http.MethodPost, "/api/container/stop?id=", nil)
	rec := httptest.NewRecorder()

	containerActionHandler(ctl, ctl.Stop).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "missing container id") {
		t.Errorf("body = %q, want missing container id message", rec.Body.String())
	}
}

func TestContainerEndpointNotFound(t *testing.T) {
	ctl := &recordingController{stopErr: ErrContainerNotFound}
	req := httptest.NewRequest(http.MethodPost, "/api/container/stop?id=missing", nil)
	rec := httptest.NewRecorder()

	containerActionHandler(ctl, ctl.Stop).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "container /api/container/stop failed") {
		t.Errorf("body = %q, want container stop failed message", rec.Body.String())
	}
}

func TestContainerEndpointControllerError(t *testing.T) {
	ctl := &recordingController{restartErr: errors.New("daemon down")}
	req := httptest.NewRequest(http.MethodPost, "/api/container/restart?id=abc", nil)
	rec := httptest.NewRecorder()

	containerActionHandler(ctl, ctl.Restart).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "container /api/container/restart failed") || !strings.Contains(body, "daemon down") {
		t.Errorf("body = %q, want container restart failed message", body)
	}
}

func TestContainerEndpointWrongMethod(t *testing.T) {
	ctl := &recordingController{}
	req := httptest.NewRequest(http.MethodGet, "/api/container/stop?id=abc", nil)
	rec := httptest.NewRecorder()

	containerActionHandler(ctl, ctl.Stop).ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
