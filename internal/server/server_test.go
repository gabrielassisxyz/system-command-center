package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthzReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	NewMuxWithHub(NewSSEHub(EmptyProvider{}, time.Hour)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRootReturnsIndexHTML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	NewMuxWithHub(NewSSEHub(EmptyProvider{}, time.Hour)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<title>hardware-usage</title>") {
		t.Errorf("GET / body missing expected title, got: %s", body)
	}
	if !strings.Contains(body, "Connecting…") {
		t.Errorf("GET / body missing expected content marker, got: %s", body)
	}
}

func TestListenAddressIsLoopback(t *testing.T) {
	want := "127.0.0.1:8765"
	if got := DefaultAddr(); got != want {
		t.Fatalf("default listen address = %q, want %q", got, want)
	}
}
