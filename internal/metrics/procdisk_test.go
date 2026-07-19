package metrics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hardware-usage/internal/model"
)

// fakeProcIOSource is a deterministic ProcIOSource for tests.
type fakeProcIOSource struct {
	stats   map[int32]ProcIOStat
	denyPID map[int32]bool
}

func (f *fakeProcIOSource) ReadProcIO(_ context.Context, pid int32) (ProcIOStat, error) {
	if f.denyPID[pid] {
		return ProcIOStat{}, errors.New("permission denied")
	}
	stat, ok := f.stats[pid]
	if !ok {
		return ProcIOStat{}, errors.New("unknown pid")
	}
	return stat, nil
}

// fakeProcIOClock steps forward by a fixed delta on each Now() call.
type fakeProcIOClock struct {
	now   time.Time
	delta time.Duration
}

func (f *fakeProcIOClock) Now() time.Time {
	cur := f.now
	f.now = f.now.Add(f.delta)
	return cur
}

func TestProcessDiskIOCollector_FirstCallYieldsAbsentRates(t *testing.T) {
	src := &fakeProcIOSource{stats: map[int32]ProcIOStat{1: {ReadBytes: 1000, WriteBytes: 2000}}}
	c := NewProcessDiskIOCollector(src)
	c.clock = &fakeProcIOClock{now: time.Unix(0, 0), delta: time.Second}

	rates := c.Collect(context.Background(), []int32{1})

	rate, ok := rates[1]
	if ok {
		if rate.Read != nil || rate.Write != nil {
			t.Errorf("first call rates should be absent, got %v", rate)
		}
	}
}

func TestProcessDiskIOCollector_SecondCallYieldsDiffedRates(t *testing.T) {
	src := &fakeProcIOSource{stats: map[int32]ProcIOStat{
		1: {ReadBytes: 1000, WriteBytes: 2000},
		2: {ReadBytes: 500, WriteBytes: 500},
	}}
	c := NewProcessDiskIOCollector(src)
	c.clock = &fakeProcIOClock{now: time.Unix(0, 0), delta: 2 * time.Second}

	c.Collect(context.Background(), []int32{1, 2})

	src.stats = map[int32]ProcIOStat{
		1: {ReadBytes: 3000, WriteBytes: 7000},
		2: {ReadBytes: 1500, WriteBytes: 2500},
	}

	rates := c.Collect(context.Background(), []int32{1, 2})

	r1 := rates[1]
	if r1.Read == nil || *r1.Read != 1000.0 {
		t.Errorf("pid 1 read rate = %v, want 1000", r1.Read)
	}
	if r1.Write == nil || *r1.Write != 2500.0 {
		t.Errorf("pid 1 write rate = %v, want 2500", r1.Write)
	}

	r2 := rates[2]
	if r2.Read == nil || *r2.Read != 500.0 {
		t.Errorf("pid 2 read rate = %v, want 500", r2.Read)
	}
	if r2.Write == nil || *r2.Write != 1000.0 {
		t.Errorf("pid 2 write rate = %v, want 1000", r2.Write)
	}
}

func TestProcessDiskIOCollector_PermissionDenied_Absent(t *testing.T) {
	src := &fakeProcIOSource{
		stats: map[int32]ProcIOStat{
			1: {ReadBytes: 1000, WriteBytes: 2000},
			2: {ReadBytes: 4000, WriteBytes: 8000},
		},
		denyPID: map[int32]bool{2: true},
	}
	c := NewProcessDiskIOCollector(src)
	c.clock = &fakeProcIOClock{now: time.Unix(0, 0), delta: time.Second}

	c.Collect(context.Background(), []int32{1, 2})

	src.stats = map[int32]ProcIOStat{
		1: {ReadBytes: 2000, WriteBytes: 3000},
		2: {ReadBytes: 8000, WriteBytes: 12000},
	}

	rates := c.Collect(context.Background(), []int32{1, 2})

	r1 := rates[1]
	if r1.Read == nil || *r1.Read != 1000.0 {
		t.Errorf("pid 1 read rate = %v, want 1000", r1.Read)
	}
	if r1.Write == nil || *r1.Write != 1000.0 {
		t.Errorf("pid 1 write rate = %v, want 1000", r1.Write)
	}

	if _, ok := rates[2]; ok {
		t.Errorf("pid 2 should be absent due to permission denied, got %v", rates[2])
	}
}

func TestProcessDiskIOCollector_NewProcessOnSecondCall_Absent(t *testing.T) {
	src := &fakeProcIOSource{stats: map[int32]ProcIOStat{1: {ReadBytes: 1000, WriteBytes: 2000}}}
	c := NewProcessDiskIOCollector(src)
	c.clock = &fakeProcIOClock{now: time.Unix(0, 0), delta: time.Second}

	c.Collect(context.Background(), []int32{1})

	src.stats = map[int32]ProcIOStat{
		1: {ReadBytes: 2000, WriteBytes: 3000},
		2: {ReadBytes: 500, WriteBytes: 500},
	}

	rates := c.Collect(context.Background(), []int32{1, 2})

	r1 := rates[1]
	if r1.Read == nil || *r1.Read != 1000.0 {
		t.Errorf("pid 1 read rate = %v, want 1000", r1.Read)
	}

	if _, ok := rates[2]; ok {
		t.Errorf("pid 2 should be absent on first appearance, got %v", rates[2])
	}
}

func TestProcessDiskIOCollector_ZeroDelta_NoDivideByZero(t *testing.T) {
	src := &fakeProcIOSource{stats: map[int32]ProcIOStat{1: {ReadBytes: 1000, WriteBytes: 2000}}}
	c := NewProcessDiskIOCollector(src)
	c.clock = &fakeProcIOClock{now: time.Unix(0, 0), delta: 0}

	c.Collect(context.Background(), []int32{1})

	src.stats = map[int32]ProcIOStat{1: {ReadBytes: 2000, WriteBytes: 4000}}

	rates := c.Collect(context.Background(), []int32{1})

	if r := rates[1]; r.Read != nil || r.Write != nil {
		t.Errorf("expected absent rates on zero delta, got %v", r)
	}
}

func TestProcessDiskIOCollector_CountersNeverDecrease(t *testing.T) {
	src := &fakeProcIOSource{stats: map[int32]ProcIOStat{1: {ReadBytes: 2000, WriteBytes: 2000}}}
	c := NewProcessDiskIOCollector(src)
	c.clock = &fakeProcIOClock{now: time.Unix(0, 0), delta: time.Second}

	c.Collect(context.Background(), []int32{1})

	// Kernel counters can sometimes appear to reset; guard against negative rates.
	src.stats = map[int32]ProcIOStat{1: {ReadBytes: 1000, WriteBytes: 1500}}

	rates := c.Collect(context.Background(), []int32{1})

	r := rates[1]
	if r.Read != nil || r.Write != nil {
		t.Errorf("expected absent rates when counters decrease, got %v", r)
	}
}

func TestProcessDiskIOCollector_MapsToModelDiskIORate(t *testing.T) {
	src := &fakeProcIOSource{stats: map[int32]ProcIOStat{1: {ReadBytes: 1000, WriteBytes: 2000}}}
	c := NewProcessDiskIOCollector(src)
	c.clock = &fakeProcIOClock{now: time.Unix(0, 0), delta: time.Second}

	c.Collect(context.Background(), []int32{1})
	src.stats = map[int32]ProcIOStat{1: {ReadBytes: 3000, WriteBytes: 5000}}

	rates := c.Collect(context.Background(), []int32{1})

	var got model.DiskIORate
	for _, r := range rates {
		got = r
	}
	if got.Read == nil || *got.Read != 2000.0 {
		t.Errorf("read rate = %v, want 2000", got.Read)
	}
	if got.Write == nil || *got.Write != 3000.0 {
		t.Errorf("write rate = %v, want 3000", got.Write)
	}
}

func TestProcFSIOSource_ReadProcIO(t *testing.T) {
	root := t.TempDir()
	pidDir := filepath.Join(root, "42")
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		t.Fatalf("create pid dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pidDir, "io"), []byte("rchar: 123\nread_bytes: 1000\nwchar: 456\nwrite_bytes: 2000\nsyscr: 1\nsyscw: 2\n"), 0644); err != nil {
		t.Fatalf("write io file: %v", err)
	}

	src := NewProcFSIOSource(root)
	stat, err := src.ReadProcIO(context.Background(), 42)
	if err != nil {
		t.Fatalf("ReadProcIO: %v", err)
	}
	if stat.ReadBytes != 1000 {
		t.Errorf("ReadBytes = %d, want 1000", stat.ReadBytes)
	}
	if stat.WriteBytes != 2000 {
		t.Errorf("WriteBytes = %d, want 2000", stat.WriteBytes)
	}
}

func TestProcFSIOSource_ReadProcIO_MissingFile(t *testing.T) {
	src := NewProcFSIOSource(t.TempDir())
	_, err := src.ReadProcIO(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for missing io file")
	}
}

func TestProcFSIOSource_ImplementsInterface(t *testing.T) {
	var _ ProcIOSource = (*ProcFSIOSource)(nil)
}
