package assembler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/shirou/gopsutil/v4/sensors"

	"hardware-usage/internal/docker"
	"hardware-usage/internal/metrics"
	"hardware-usage/internal/model"
)

func u64(v uint64) *uint64   { return &v }
func f64(v float64) *float64 { return &v }

// fakeSystemSource returns a deterministic system snapshot.
type fakeSystemSource struct {
	absent bool
	cpu    float64
	ram    mem.VirtualMemoryStat
	temp   []sensors.TemperatureStat
}

func (f fakeSystemSource) CPUPercent(_ context.Context) ([]float64, error) {
	if f.absent {
		return nil, nil
	}
	return []float64{f.cpu}, nil
}

func (f fakeSystemSource) VirtualMemory(_ context.Context) (*mem.VirtualMemoryStat, error) {
	if f.absent {
		return nil, nil
	}
	return &f.ram, nil
}

func (f fakeSystemSource) SensorsTemperatures(_ context.Context) ([]sensors.TemperatureStat, error) {
	if f.absent {
		return nil, nil
	}
	return f.temp, nil
}

// fakeIOSource returns cumulative counters. Fields are mutated by tests.
type fakeIOSource struct {
	disk map[string]disk.IOCountersStat
	net  []net.IOCountersStat
}

func (f fakeIOSource) DiskIOCounters(_ context.Context) (map[string]disk.IOCountersStat, error) {
	return f.disk, nil
}

func (f fakeIOSource) NetIOCounters(_ context.Context) ([]net.IOCountersStat, error) {
	return f.net, nil
}

// fakeProcessSource returns the configured processes.
type fakeProcessSource struct {
	procs []fakeProcess
}

type fakeProcess struct {
	pid  int32
	name string
	cpu  float64
	rss  uint64
}

func (f fakeProcessSource) Processes(_ context.Context) ([]*process.Process, error) {
	out := make([]*process.Process, len(f.procs))
	for i, p := range f.procs {
		out[i] = &process.Process{Pid: p.pid}
	}
	return out, nil
}

func (f fakeProcessSource) Info(_ context.Context, p *process.Process) (metrics.ProcessInfo, error) {
	for _, fp := range f.procs {
		if fp.pid == p.Pid {
			return metrics.ProcessInfo{
				PID:  fp.pid,
				Name: fp.name,
				CPU:  fp.cpu,
				RAM:  process.MemoryInfoStat{RSS: fp.rss},
			}, nil
		}
	}
	return metrics.ProcessInfo{}, nil
}

// fakeProcIOSource returns cumulative I/O counters per pid.
type fakeProcIOSource struct {
	stats map[int32]metrics.ProcIOStat
}

func (f fakeProcIOSource) ReadProcIO(_ context.Context, pid int32) (metrics.ProcIOStat, error) {
	return f.stats[pid], nil
}

// fakeGPUReader ignores the context and returns a constant GPU snapshot.
type fakeGPUReader struct {
	info model.GPUInfo
}

func (f fakeGPUReader) Read(_ context.Context) model.GPUInfo {
	return f.info
}

// fakeDockerSource returns containers and stats from fixed maps.
type fakeDockerSource struct {
	containers []docker.ContainerSummary
	stats      map[string]*docker.ContainerStats
}

func (f fakeDockerSource) ContainerList(_ context.Context) ([]docker.ContainerSummary, error) {
	return f.containers, nil
}

func (f fakeDockerSource) ContainerStats(_ context.Context, id string) (*docker.ContainerStats, error) {
	return f.stats[id], nil
}

func newTestAssembler(t *testing.T, tc *testClock, sys fakeSystemSource, io fakeIOSource, procs fakeProcessSource, procIO fakeProcIOSource, g fakeGPUReader, dc fakeDockerSource) *Assembler {
	t.Helper()
	return NewWithClock(tc, sys, io, procs, procIO, &g, &dc)
}

func TestAssemblerBuildsCompleteSnapshot(t *testing.T) {
	tc := &testClock{t: time.Unix(0, 0)}
	sys := fakeSystemSource{
		cpu: 25.0,
		ram: mem.VirtualMemoryStat{Used: 8 << 30, Total: 16 << 30},
		temp: []sensors.TemperatureStat{
			{SensorKey: "t1", Temperature: 55},
		},
	}
	io := fakeIOSource{
		disk: map[string]disk.IOCountersStat{"sda": {ReadBytes: 1000, WriteBytes: 2000}},
		net:  []net.IOCountersStat{{Name: "eth0", BytesRecv: 3000, BytesSent: 4000}},
	}
	procs := fakeProcessSource{procs: []fakeProcess{
		{pid: 1, name: "init", cpu: 1.5, rss: 1 << 20},
		{pid: 7, name: "cron", cpu: 0.5, rss: 2 << 20},
	}}
	procIO := fakeProcIOSource{stats: map[int32]metrics.ProcIOStat{
		1: {ReadBytes: 100, WriteBytes: 200},
		7: {ReadBytes: 10, WriteBytes: 20},
	}}
	g := fakeGPUReader{info: model.GPUInfo{Busy: f64(30), VRAMUsed: u64(1 << 30), VRAMTotal: u64(8 << 30), Temp: f64(45)}}
	dc := fakeDockerSource{containers: []docker.ContainerSummary{
		{ID: "c1", Names: []string{"/core_app"}, Labels: map[string]string{"com.docker.compose.project": "core"}},
	}, stats: map[string]*docker.ContainerStats{
		"c1": {
			CPUStats:    docker.CPUStats{CPUUsage: docker.CPUUsage{TotalUsage: 1e9}, SystemUsage: 4e9, OnlineCPUs: 4},
			PreCPUStats: docker.CPUStats{CPUUsage: docker.CPUUsage{TotalUsage: 0}, SystemUsage: 0, OnlineCPUs: 4},
			MemoryStats: docker.MemoryStats{Usage: 128 << 20, Limit: 1 << 30},
		},
	}}

	a := newTestAssembler(t, tc, sys, io, procs, procIO, g, dc)
	a.clock = tc

	// Docker is served from a background-refreshed cache; prime it so Collect
	// sees the fake source's containers rather than an empty cache.
	a.RefreshDocker(context.Background())
	snap := a.Collect(context.Background())

	if got := *snap.System.CPU; got != 25.0 {
		t.Errorf("System.CPU = %v, want 25.0", got)
	}
	if snap.System.RAM == nil || *snap.System.RAM.Used != 8<<30 || *snap.System.RAM.Total != 16<<30 {
		t.Errorf("System.RAM = %+v, want used=8GiB total=16GiB", snap.System.RAM)
	}
	if snap.System.GPU == nil || snap.System.GPU.Busy == nil || *snap.System.GPU.Busy != 30 {
		t.Errorf("System.GPU.Busy missing or wrong: %+v", snap.System.GPU)
	}
	if len(snap.Processes) != 2 {
		t.Fatalf("Processes = %d rows, want 2", len(snap.Processes))
	}
	if snap.Processes[0].DiskIO != nil {
		t.Errorf("first process DiskIO should be absent, got %+v", snap.Processes[0].DiskIO)
	}
	if len(snap.Docker) != 1 || snap.Docker[0].Project != "core" {
		t.Errorf("Docker groups wrong: %+v", snap.Docker)
	}

	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"\"system\"", "\"processes\"", "\"docker\""} {
		if !strings.Contains(string(b), key) {
			t.Errorf("snapshot JSON missing %s", key)
		}
	}
}

func TestAssemblerRatesAdvanceOnSecondCall(t *testing.T) {
	tc := &testClock{t: time.Unix(0, 0)}
	sys := fakeSystemSource{cpu: 10.0, ram: mem.VirtualMemoryStat{Used: 1, Total: 2}}
	io := fakeIOSource{
		disk: map[string]disk.IOCountersStat{"sda": {ReadBytes: 1000, WriteBytes: 2000}},
		net:  []net.IOCountersStat{{Name: "eth0", BytesRecv: 3000, BytesSent: 4000}},
	}
	procs := fakeProcessSource{procs: []fakeProcess{{pid: 1, name: "init", cpu: 1, rss: 1 << 20}}}
	procIO := fakeProcIOSource{stats: map[int32]metrics.ProcIOStat{1: {ReadBytes: 100, WriteBytes: 200}}}
	g := fakeGPUReader{}
	dc := fakeDockerSource{containers: []docker.ContainerSummary{
		{ID: "c1", Names: []string{"/app"}, Labels: map[string]string{"com.docker.compose.project": "core"}},
	}, stats: map[string]*docker.ContainerStats{
		"c1": {
			CPUStats:    docker.CPUStats{CPUUsage: docker.CPUUsage{TotalUsage: 1e9}, SystemUsage: 4e9, OnlineCPUs: 4},
			PreCPUStats: docker.CPUStats{CPUUsage: docker.CPUUsage{TotalUsage: 0}, SystemUsage: 0, OnlineCPUs: 4},
			MemoryStats: docker.MemoryStats{Usage: 128 << 20, Limit: 1 << 30},
		},
	}}

	a := newTestAssembler(t, tc, sys, io, procs, procIO, g, dc)
	_ = a.Collect(context.Background())

	// Advance counters and clock.
	tc.t = tc.t.Add(2 * time.Second)
	io.disk["sda"] = disk.IOCountersStat{ReadBytes: 3000, WriteBytes: 7000}
	io.net[0] = net.IOCountersStat{Name: "eth0", BytesRecv: 9000, BytesSent: 12000}
	procIO.stats[1] = metrics.ProcIOStat{ReadBytes: 500, WriteBytes: 800}

	snap := a.Collect(context.Background())

	if snap.System.DiskIO == nil {
		t.Fatal("System.DiskIO absent on second call")
	}
	if got := *snap.System.DiskIO.Read; got != 1000 {
		t.Errorf("System.DiskIO.Read = %v, want 1000", got)
	}
	if got := *snap.System.DiskIO.Write; got != 2500 {
		t.Errorf("System.DiskIO.Write = %v, want 2500", got)
	}
	if snap.System.NetIO == nil {
		t.Fatal("System.NetIO absent on second call")
	}
	if got := *snap.System.NetIO.Rx; got != 3000 {
		t.Errorf("System.NetIO.Rx = %v, want 3000", got)
	}
	if got := *snap.System.NetIO.Tx; got != 4000 {
		t.Errorf("System.NetIO.Tx = %v, want 4000", got)
	}
	if len(snap.Processes) != 1 {
		t.Fatalf("Processes = %d, want 1", len(snap.Processes))
	}
	row := snap.Processes[0]
	if row.DiskIO == nil {
		t.Fatal("Process.DiskIO absent on second call")
	}
	if got := *row.DiskIO.Read; got != 200 {
		t.Errorf("Process.DiskIO.Read = %v, want 200", got)
	}
	if got := *row.DiskIO.Write; got != 300 {
		t.Errorf("Process.DiskIO.Write = %v, want 300", got)
	}
}

func TestAssemblerSnapshotProvider(t *testing.T) {
	tc := &testClock{t: time.Unix(0, 0)}
	a := newTestAssembler(t, tc,
		fakeSystemSource{absent: true},
		fakeIOSource{},
		fakeProcessSource{},
		fakeProcIOSource{},
		fakeGPUReader{},
		fakeDockerSource{},
	)

	snap := a.Snapshot()
	if snap.System.CPU != nil || snap.System.RAM != nil {
		t.Errorf("empty assembler should produce absent system metrics, got %+v", snap.System)
	}
}

// testClock is a deterministic clock for tests.
type testClock struct {
	t time.Time
}

func (c *testClock) Now() time.Time { return c.t }
