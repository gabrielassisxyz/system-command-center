package metrics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hardware-usage/internal/model"
)

// ProcIOStat holds the cumulative read/write byte counters parsed from
// /proc/<pid>/io for a single process.
type ProcIOStat struct {
	ReadBytes  uint64
	WriteBytes uint64
}

// ProcIOSource abstracts reading per-process I/O counters from procfs. Tests
// inject a fake so collection never touches real /proc/<pid>/io.
type ProcIOSource interface {
	ReadProcIO(ctx context.Context, pid int32) (ProcIOStat, error)
}

// ProcFSIOSource is the production ProcIOSource backed by /proc.
type ProcFSIOSource struct {
	root string
}

// NewProcFSIOSource creates a source rooted at root, usually "/proc".
func NewProcFSIOSource(root string) *ProcFSIOSource {
	return &ProcFSIOSource{root: root}
}

// ReadProcIO parses read_bytes and write_bytes from /proc/<pid>/io.
func (s *ProcFSIOSource) ReadProcIO(_ context.Context, pid int32) (ProcIOStat, error) {
	var stat ProcIOStat
	data, err := os.ReadFile(filepath.Join(s.root, fmt.Sprintf("%d", pid), "io"))
	if err != nil {
		return stat, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		val, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			continue
		}
		switch parts[0] {
		case "read_bytes:":
			stat.ReadBytes = val
		case "write_bytes:":
			stat.WriteBytes = val
		}
	}
	return stat, nil
}

// ioClock abstracts time for rate calculations so tests can control the
// interval between two Collect() calls deterministically.
type ioClock interface {
	Now() time.Time
}

// realIOClock is the production ioClock backed by time.Now().
type realIOClock struct{}

func (realIOClock) Now() time.Time { return time.Now() }

// procIOSample holds a single point-in-time reading of per-process cumulative
// I/O counters.
type procIOSample struct {
	time  time.Time
	stats map[int32]ProcIOStat
}

// ProcessDiskIOCollector turns cumulative per-process I/O counters into byte
// rates. It is stateful: the first Collect() call stores a baseline and
// returns no disk rates; subsequent calls return delta / deltaT.
type ProcessDiskIOCollector struct {
	src   ProcIOSource
	clock ioClock
	prev  *procIOSample
}

// NewProcessDiskIOCollector creates a collector backed by src and the real clock.
func NewProcessDiskIOCollector(src ProcIOSource) *ProcessDiskIOCollector {
	return NewProcessDiskIOCollectorWithClock(src, realIOClock{})
}

// NewProcessDiskIOCollectorWithClock creates a collector with an explicit clock.
func NewProcessDiskIOCollectorWithClock(src ProcIOSource, clock ioClock) *ProcessDiskIOCollector {
	return &ProcessDiskIOCollector{src: src, clock: clock}
}

// Collect returns diffed read/write disk I/O rates for the supplied pids.
// A permission-denied or missing read for a pid leaves that pid's rates absent
// rather than surfacing an error. The first call always returns all rates
// absent because there is no previous sample to diff against.
func (c *ProcessDiskIOCollector) Collect(ctx context.Context, pids []int32) map[int32]model.DiskIORate {
	now := c.clock.Now()
	stats := make(map[int32]ProcIOStat, len(pids))
	for _, pid := range pids {
		stat, err := c.src.ReadProcIO(ctx, pid)
		if err != nil {
			continue
		}
		stats[pid] = stat
	}

	// Copy the source data so a caller that mutates returned maps cannot
	// corrupt the previous sample we diff against next time.
	statsCopy := make(map[int32]ProcIOStat, len(stats))
	for pid, stat := range stats {
		statsCopy[pid] = stat
	}

	sample := &procIOSample{time: now, stats: statsCopy}
	rates := make(map[int32]model.DiskIORate, len(pids))

	if c.prev != nil {
		dt := sample.time.Sub(c.prev.time).Seconds()
		if dt > 0 {
			for pid, cur := range sample.stats {
				prev, ok := c.prev.stats[pid]
				if !ok {
					continue
				}
				var rate model.DiskIORate
				if cur.ReadBytes >= prev.ReadBytes {
					r := float64(cur.ReadBytes-prev.ReadBytes) / dt
					rate.Read = &r
				}
				if cur.WriteBytes >= prev.WriteBytes {
					w := float64(cur.WriteBytes-prev.WriteBytes) / dt
					rate.Write = &w
				}
				rates[pid] = rate
			}
		}
	}

	c.prev = sample
	return rates
}
