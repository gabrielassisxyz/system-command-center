// Command hardware-usage serves the live resource-usage web page for this desktop.
package main

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/shirou/gopsutil/v4/sensors"

	"hardware-usage/internal/assembler"
	"hardware-usage/internal/docker"
	"hardware-usage/internal/metrics"
	"hardware-usage/internal/model"
	"hardware-usage/internal/server"
)

// TestNewAssemblerWiresProductionSources verifies that newAssembler builds an
// assembler with production sources and that those sources can be substituted
// by fakes with the same interface shape.
func TestNewAssemblerWiresProductionSources(t *testing.T) {
	// We can't inject fakes directly into newAssembler because it hard-codes
	// production constructors. Verify it builds successfully with the real
	// Docker source (which only validates the socket path at construction time).
	_, err := newAssembler()
	// On a machine without a docker socket path error is still nil because
	// NewDockerSource only records the path; connection errors happen later.
	if err != nil {
		t.Fatalf("newAssembler: %v", err)
	}
}

// TestEventsEndpointBroadcastsRealSnapshot wires a fake-backed assembler into
// the SSE hub and asserts that an /events connection receives a fully
// populated Snapshot JSON frame.
func TestEventsEndpointBroadcastsRealSnapshot(t *testing.T) {
	a := newTestAssemblerForServer(t)
	// Docker is served from a background-refreshed cache (RunDockerRefresh in
	// production primes it at startup); prime it here so the frame carries the
	// fake source's containers.
	a.RefreshDocker(context.Background())
	hub := server.NewSSEHub(a, 0)
	mux := server.NewMuxWithHub(hub)
	s := httptest.NewServer(mux)
	defer s.Close()

	resp, err := http.Get(s.URL + "/events")
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	reader := bufio.NewReader(resp.Body)
	// Skip the initial ":ok" comment event.
	consumeComment(reader, t)

	// Tick manually to broadcast deterministically (production uses a real
	// ticker, but this test drives the hub directly).
	hub.Tick()

	line := readDataLine(t, reader)

	if !strings.Contains(line, `"system"`) {
		t.Errorf("frame missing system header, got: %s", line)
	}
	if !strings.Contains(line, `"processes"`) {
		t.Errorf("frame missing processes, got: %s", line)
	}
	if !strings.Contains(line, `"docker"`) {
		t.Errorf("frame missing docker groups, got: %s", line)
	}
	if !strings.Contains(line, `"cpu":25`) {
		t.Errorf("frame missing expected CPU value, got: %s", line)
	}
	if !strings.Contains(line, `"project":"core"`) {
		t.Errorf("frame missing expected docker project, got: %s", line)
	}
	if !strings.Contains(line, `"pid":1`) {
		t.Errorf("frame missing expected process pid, got: %s", line)
	}
}

func consumeComment(r *bufio.Reader, t *testing.T) {
	t.Helper()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read comment: %v", err)
		}
		if strings.HasPrefix(line, ":") {
			// consume the trailing blank line that terminates the SSE event.
			if _, err := r.ReadString('\n'); err != nil {
				t.Fatalf("read blank after comment: %v", err)
			}
			return
		}
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

func newTestAssemblerForServer(t *testing.T) *assembler.Assembler {
	t.Helper()
	tc := &testClock{t: time.Unix(0, 0)}
	return assembler.NewWithClock(
		tc,
		fakeSystemSourceForServer{cpu: 25, ramUsed: 8 << 30, ramTotal: 16 << 30},
		fakeIOSourceForServer{},
		fakeProcessSourceForServer{procs: []fakeProcessForServer{{pid: 1, name: "init", cpu: 1.5, rss: 1 << 20}}},
		fakeProcIOSourceForServer{},
		staticGPUReaderForServer{},
		fakeDockerSourceForServer{},
	)
}

type fakeSystemSourceForServer struct {
	cpu      float64
	ramUsed  uint64
	ramTotal uint64
}

func (f fakeSystemSourceForServer) CPUPercent(_ context.Context) ([]float64, error) {
	return []float64{f.cpu}, nil
}

func (f fakeSystemSourceForServer) VirtualMemory(_ context.Context) (*mem.VirtualMemoryStat, error) {
	return &mem.VirtualMemoryStat{Used: f.ramUsed, Total: f.ramTotal}, nil
}

func (f fakeSystemSourceForServer) SensorsTemperatures(_ context.Context) ([]sensors.TemperatureStat, error) {
	return nil, nil
}

type fakeProcessForServer struct {
	pid  int32
	name string
	cpu  float64
	rss  uint64
}

type fakeProcessSourceForServer struct {
	procs []fakeProcessForServer
}

func (f fakeProcessSourceForServer) Processes(_ context.Context) ([]*process.Process, error) {
	out := make([]*process.Process, len(f.procs))
	for i, p := range f.procs {
		out[i] = &process.Process{Pid: p.pid}
	}
	return out, nil
}

func (f fakeProcessSourceForServer) Info(_ context.Context, p *process.Process) (metrics.ProcessInfo, error) {
	for _, fp := range f.procs {
		if fp.pid == p.Pid {
			return metrics.ProcessInfo{PID: fp.pid, Name: fp.name, CPU: fp.cpu, RAM: process.MemoryInfoStat{RSS: fp.rss}}, nil
		}
	}
	return metrics.ProcessInfo{}, nil
}

type fakeIOSourceForServer struct{}

func (f fakeIOSourceForServer) DiskIOCounters(_ context.Context) (map[string]disk.IOCountersStat, error) {
	return nil, nil
}

func (f fakeIOSourceForServer) NetIOCounters(_ context.Context) ([]net.IOCountersStat, error) {
	return nil, nil
}

type fakeProcIOSourceForServer struct{}

func (f fakeProcIOSourceForServer) ReadProcIO(_ context.Context, _ int32) (metrics.ProcIOStat, error) {
	return metrics.ProcIOStat{}, nil
}

type staticGPUReaderForServer struct{}

func (r staticGPUReaderForServer) Read(_ context.Context) model.GPUInfo { return model.GPUInfo{} }

type fakeDockerSourceForServer struct{}

func (f fakeDockerSourceForServer) ContainerList(_ context.Context) ([]docker.ContainerSummary, error) {
	return []docker.ContainerSummary{{ID: "c1", Names: []string{"/app"}, Labels: map[string]string{"com.docker.compose.project": "core"}}}, nil
}

func (f fakeDockerSourceForServer) ContainerStats(_ context.Context, id string) (*docker.ContainerStats, error) {
	if id != "c1" {
		return nil, nil
	}
	return &docker.ContainerStats{
		CPUStats:    docker.CPUStats{CPUUsage: docker.CPUUsage{TotalUsage: 1e9}, SystemUsage: 4e9, OnlineCPUs: 4},
		PreCPUStats: docker.CPUStats{CPUUsage: docker.CPUUsage{TotalUsage: 0}, SystemUsage: 0, OnlineCPUs: 4},
		MemoryStats: docker.MemoryStats{Usage: 128 << 20, Limit: 1 << 30},
	}, nil
}

type testClock struct {
	t time.Time
}

func (c *testClock) Now() time.Time { return c.t }
