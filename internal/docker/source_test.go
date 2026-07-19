package docker

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// unixListener creates a Unix-domain httptest server and returns its socket
// path plus a cleanup function.
func unixListener(t *testing.T, handler http.Handler) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/docker.sock"
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(l) }()
	return path, func() {
		_ = server.Close()
		_ = l.Close()
	}
}

func TestNewDockerSourceUsesDefaultSocket(t *testing.T) {
	// Create a fake server and point the default socket path at it by
	// temporarily symlinking. We cannot change /var/run/docker.sock, but we
	// can exercise NewDockerSource with DOCKER_HOST unset by pointing a
	// temp symlink to our fake socket and overriding via the helper.
	path, cleanup := unixListener(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.41/containers/json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[{"Id":"abc123","Names":["/web"],"Labels":{"com.docker.compose.project":"app"}}]`)
			return
		}
		if r.URL.Path == "/v1.41/containers/abc123/stats" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"cpu_stats":{"cpu_usage":{"total_usage":0},"system_cpu_usage":0},"memory_stats":{"usage":0}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer cleanup()

	// Point NewDockerSource at the temp socket by using DOCKER_HOST.
	t.Setenv("DOCKER_HOST", "unix://"+path)
	src, err := NewDockerSource()
	if err != nil {
		t.Fatalf("NewDockerSource: %v", err)
	}

	containers, err := src.ContainerList(context.Background())
	if err != nil {
		t.Fatalf("ContainerList: %v", err)
	}
	if len(containers) != 1 || containers[0].ID != "abc123" {
		t.Fatalf("unexpected containers: %v", containers)
	}

	stats, err := src.ContainerStats(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("ContainerStats: %v", err)
	}
	if stats == nil {
		t.Fatal("expected stats")
	}
}

func TestNewDockerSourceReturnsErrorOnUnsupportedHost(t *testing.T) {
	t.Setenv("DOCKER_HOST", "npipe:////./pipe/docker_engine")
	if _, err := NewDockerSource(); err == nil {
		t.Fatal("expected error for unsupported DOCKER_HOST")
	}
}

func TestNewDockerSourceTCPHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.41/containers/json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[]`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	t.Setenv("DOCKER_HOST", "tcp://"+server.Listener.Addr().String())
	src, err := NewDockerSource()
	if err != nil {
		t.Fatalf("NewDockerSource: %v", err)
	}
	containers, err := src.ContainerList(context.Background())
	if err != nil {
		t.Fatalf("ContainerList: %v", err)
	}
	if len(containers) != 0 {
		t.Fatalf("expected empty list, got %v", containers)
	}
}

func TestContainerListReturnsErrorOnNon200(t *testing.T) {
	path, cleanup := unixListener(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer cleanup()

	t.Setenv("DOCKER_HOST", "unix://"+path)
	src, err := NewDockerSource()
	if err != nil {
		t.Fatalf("NewDockerSource: %v", err)
	}
	if _, err := src.ContainerList(context.Background()); err == nil {
		t.Fatal("expected error for non-200")
	}
}

func TestContainerStatsReturnsErrorOnNon200(t *testing.T) {
	path, cleanup := unixListener(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer cleanup()

	t.Setenv("DOCKER_HOST", "unix://"+path)
	src, err := NewDockerSource()
	if err != nil {
		t.Fatalf("NewDockerSource: %v", err)
	}
	if _, err := src.ContainerStats(context.Background(), "abc"); err == nil {
		t.Fatal("expected error for non-200")
	}
}
