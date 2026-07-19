package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/shirou/gopsutil/v4/process"

	"hardware-usage/internal/model"
)

// fakeProcessSource is a deterministic ProcessSource for tests.
type fakeProcessSource struct {
	procs      []*process.Process
	infoByPID  map[int32]ProcessInfo
	infoErrPID map[int32]bool
	calledInfo map[int32]bool
	procErr    error
}

func newFakeProcessSource() *fakeProcessSource {
	return &fakeProcessSource{
		infoByPID:  make(map[int32]ProcessInfo),
		infoErrPID: make(map[int32]bool),
		calledInfo: make(map[int32]bool),
	}
}

func (f *fakeProcessSource) add(p ProcessInfo) {
	f.procs = append(f.procs, &process.Process{Pid: p.PID})
	f.infoByPID[p.PID] = p
}

func (f *fakeProcessSource) addErr(pid int32) {
	f.procs = append(f.procs, &process.Process{Pid: pid})
	f.infoErrPID[pid] = true
}

func (f *fakeProcessSource) Processes(ctx context.Context) ([]*process.Process, error) {
	return f.procs, f.procErr
}

func (f *fakeProcessSource) Info(ctx context.Context, p *process.Process) (ProcessInfo, error) {
	f.calledInfo[p.Pid] = true
	if f.infoErrPID[p.Pid] {
		return ProcessInfo{}, errors.New("info failed")
	}
	info, ok := f.infoByPID[p.Pid]
	if !ok {
		return ProcessInfo{}, errors.New("unknown pid")
	}
	return info, nil
}

func TestProcessCollector_FullyPopulated(t *testing.T) {
	src := newFakeProcessSource()
	src.add(ProcessInfo{PID: 1, Name: "init", CPU: 0.5, RAM: process.MemoryInfoStat{RSS: 4 << 20}})
	src.add(ProcessInfo{PID: 42, Name: "worker", CPU: 12.5, RAM: process.MemoryInfoStat{RSS: 256 << 20}})

	c := NewProcessCollector(src)
	rows := c.Collect(context.Background())

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	assertRow(t, rows[0], 1, "init", 0.5, 4<<20)
	assertRow(t, rows[1], 42, "worker", 12.5, 256<<20)
}

func TestProcessCollector_MetricReadErrors_IndividualRowsAbsent(t *testing.T) {
	src := newFakeProcessSource()
	src.add(ProcessInfo{PID: 1, Name: "good", CPU: 1.0, RAM: process.MemoryInfoStat{RSS: 1 << 20}})
	src.addErr(2)
	src.add(ProcessInfo{PID: 3, Name: "also-good", CPU: 3.0, RAM: process.MemoryInfoStat{RSS: 3 << 20}})

	c := NewProcessCollector(src)
	rows := c.Collect(context.Background())

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	assertRow(t, rows[0], 1, "good", 1.0, 1<<20)
	assertRow(t, rows[1], 3, "also-good", 3.0, 3<<20)

	if !src.calledInfo[2] {
		t.Errorf("expected info to be attempted for pid 2")
	}
}

func TestProcessCollector_SourceError_ReturnsEmpty(t *testing.T) {
	src := newFakeProcessSource()
	src.add(ProcessInfo{PID: 1, Name: "init", CPU: 0.5, RAM: process.MemoryInfoStat{RSS: 1 << 20}})
	src.procErr = errors.New("cannot list processes")

	c := NewProcessCollector(src)
	rows := c.Collect(context.Background())

	if len(rows) != 0 {
		t.Fatalf("expected empty rows on process-list error, got %d", len(rows))
	}
}

func TestProcessCollector_UnnamedProcessSkipped(t *testing.T) {
	src := newFakeProcessSource()
	src.add(ProcessInfo{PID: 1, Name: "init", CPU: 0.5, RAM: process.MemoryInfoStat{RSS: 1 << 20}})
	src.add(ProcessInfo{PID: 2, Name: "", CPU: 99.0, RAM: process.MemoryInfoStat{RSS: 1 << 30}})

	c := NewProcessCollector(src)
	rows := c.Collect(context.Background())

	if len(rows) != 1 {
		t.Fatalf("expected unnamed process to be skipped, got %d rows", len(rows))
	}
	if rows[0].PID != 1 {
		t.Errorf("expected pid 1, got %d", rows[0].PID)
	}
}

func TestProcessCollector_ZeroRAM_Omitted(t *testing.T) {
	src := newFakeProcessSource()
	src.add(ProcessInfo{PID: 1, Name: "ghost", CPU: 5.0, RAM: process.MemoryInfoStat{RSS: 0}})

	c := NewProcessCollector(src)
	rows := c.Collect(context.Background())

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RAM != nil {
		t.Errorf("expected RAM to be omitted when RSS is zero, got %v", rows[0].RAM)
	}
}

func TestGopsutilProcessSource_ImplementsInterface(t *testing.T) {
	var _ ProcessSource = GopsutilProcessSource{}
}

func TestFakeProcessSource_ImplementsInterface(t *testing.T) {
	var _ ProcessSource = (*fakeProcessSource)(nil)
}

func assertRow(t *testing.T, got model.ProcessRow, wantPID int32, wantName string, wantCPU float64, wantRAM uint64) {
	t.Helper()
	if got.PID != wantPID {
		t.Errorf("PID = %d, want %d", got.PID, wantPID)
	}
	if got.Name != wantName {
		t.Errorf("Name = %q, want %q", got.Name, wantName)
	}
	if got.CPU == nil || *got.CPU != wantCPU {
		t.Errorf("CPU = %v, want %v", got.CPU, wantCPU)
	}
	if got.RAM == nil || got.RAM.Used == nil || *got.RAM.Used != wantRAM {
		t.Errorf("RAM.Used = %v, want %v", got.RAM, wantRAM)
	}
}
