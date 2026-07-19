package server

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"hardware-usage/internal/model"
)

func floatPtr(v float64) *float64 { return &v }

type countingProvider struct {
	mu    sync.Mutex
	calls int
	snap  model.Snapshot
}

func (p *countingProvider) Snapshot() model.Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.snap
}

func (p *countingProvider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func readOKComment(t *testing.T, r *bufio.Reader) {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read initial comment: %v", err)
	}
	if line != ":ok\n" {
		t.Fatalf("expected :ok comment, got %q", line)
	}
	// Consume the blank line that terminates the SSE event.
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read blank after comment: %v", err)
	}
}

func readDataLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE line: %v", err)
		}
		if strings.HasPrefix(line, "data:") {
			return line
		}
	}
}

func TestHubBroadcastsSnapshotToTwoClients(t *testing.T) {
	p := &countingProvider{snap: model.Snapshot{
		System: model.SystemSnapshot{CPU: floatPtr(12.5)},
	}}
	hub := NewSSEHub(p, time.Hour)

	s := httptest.NewServer(hub)
	defer s.Close()

	resp1, err := http.Get(s.URL)
	if err != nil {
		t.Fatalf("client 1 connect: %v", err)
	}
	defer func() { _ = resp1.Body.Close() }()
	resp2, err := http.Get(s.URL)
	if err != nil {
		t.Fatalf("client 2 connect: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()

	r1 := bufio.NewReader(resp1.Body)
	r2 := bufio.NewReader(resp2.Body)
	readOKComment(t, r1)
	readOKComment(t, r2)

	hub.Tick()

	line1 := readDataLine(t, r1)
	line2 := readDataLine(t, r2)

	if !strings.Contains(line1, `"cpu":12.5`) {
		t.Errorf("client 1 frame missing cpu, got: %s", line1)
	}
	if !strings.Contains(line2, `"cpu":12.5`) {
		t.Errorf("client 2 frame missing cpu, got: %s", line2)
	}
	if p.Calls() != 1 {
		t.Errorf("provider called %d times for one tick, want 1", p.Calls())
	}
}

func TestHubDoesNotCallProviderAfterDisconnect(t *testing.T) {
	p := &countingProvider{snap: model.Snapshot{}}
	hub := NewSSEHub(p, time.Hour)

	s := httptest.NewServer(hub)
	defer s.Close()

	resp, err := http.Get(s.URL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	r := bufio.NewReader(resp.Body)
	readOKComment(t, r)

	hub.Tick()
	if p.Calls() != 1 {
		t.Fatalf("first tick calls = %d, want 1", p.Calls())
	}

	_ = resp.Body.Close()

	// Wait for the server to notice the disconnect and unregister the client.
	for i := 0; i < 50; i++ {
		if !hub.hasClients() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if hub.hasClients() {
		t.Fatal("client still registered after disconnect")
	}

	hub.Tick()
	if p.Calls() != 1 {
		t.Fatalf("provider called after disconnect: got %d calls", p.Calls())
	}
}

func TestHubDoesNotCallProviderWithoutSubscribers(t *testing.T) {
	p := &countingProvider{snap: model.Snapshot{}}
	hub := NewSSEHub(p, time.Hour)

	hub.Tick()

	if p.Calls() != 0 {
		t.Fatalf("provider called with no subscribers: got %d calls", p.Calls())
	}
}
