// Package assembler combines all metric collectors into a single
// model.Snapshot, holding the previous sample so it can compute diff-based
// I/O rates on each assembly.
package assembler

import (
	"context"
	"time"

	"hardware-usage/internal/docker"
	"hardware-usage/internal/metrics"
	"hardware-usage/internal/model"
)

// Clock abstracts time so tests can control the interval between assemblies.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// GPUReader reads GPU metrics. This interface keeps the assembler decoupled
// from the GPU package so tests can inject a fake reader without touching
// sysfs.
type GPUReader interface {
	Read(ctx context.Context) model.GPUInfo
}

// Assembler is the single SnapshotProvider used by the SSE hub. It owns all
// collectors and the state needed to turn cumulative counters into rates.
type Assembler struct {
	system     *metrics.SystemCollector
	io         *metrics.IORateCollector
	processes  *metrics.ProcessCollector
	procDiskIO *metrics.ProcessDiskIOCollector
	gpu        GPUReader
	docker     *docker.DockerCollector
	clock      Clock
}

// NewWithClock builds an Assembler with an explicit clock. Tests use this to
// control the interval between assemblies; production callers should use New.
func NewWithClock(
	clock Clock,
	sysSrc metrics.SystemSource,
	ioSrc metrics.IOSource,
	procSrc metrics.ProcessSource,
	procIOSrc metrics.ProcIOSource,
	gpuReader GPUReader,
	dockerSrc docker.DockerSource,
) *Assembler {
	a := New(sysSrc, ioSrc, procSrc, procIOSrc, gpuReader, dockerSrc)
	a.clock = clock
	// Replace sub-collector clocks with the same deterministic clock so all
	// diff-based collectors share the same timeline. The same ioClockAdapter
	// value satisfies both metrics.IOClock and metrics.ioClock.
	ioClock := ioClockAdapter{clock}
	a.io = metrics.NewIORateCollectorWithClock(ioSrc, ioClock)
	a.procDiskIO = metrics.NewProcessDiskIOCollectorWithClock(procIOSrc, ioClock)
	return a
}

// ioClockAdapter adapts the assembler's Clock interface to metrics.IOClock and
// metrics.ioClock. Both only expose Now(), so one adapter works for both.
type ioClockAdapter struct {
	clock Clock
}

func (a ioClockAdapter) Now() time.Time { return a.clock.Now() }

// New builds an Assembler from production collectors. The supplied source
// objects are wired directly; the caller controls which implementations are
// used, so tests can inject fakes.
func New(
	sysSrc metrics.SystemSource,
	ioSrc metrics.IOSource,
	procSrc metrics.ProcessSource,
	procIOSrc metrics.ProcIOSource,
	gpuReader GPUReader,
	dockerSrc docker.DockerSource,
) *Assembler {
	return &Assembler{
		system:     metrics.NewSystemCollector(sysSrc),
		io:         metrics.NewIORateCollector(ioSrc),
		processes:  metrics.NewProcessCollector(procSrc),
		procDiskIO: metrics.NewProcessDiskIOCollector(procIOSrc),
		gpu:        gpuReader,
		docker:     docker.NewDockerCollector(dockerSrc),
		clock:      realClock{},
	}
}

// Collect gathers a full snapshot from every collector, diffs rates against
// the previous sample, and stores the result for the next call. It is safe to
// call from the SSE hub's broadcast tick. The first call always yields zero
// (or absent) rates because there is no previous sample.
func (a *Assembler) Collect(ctx context.Context) model.Snapshot {
	var snap model.Snapshot

	snap.System = a.system.Collect(ctx)
	diskRate, netRate := a.io.Collect(ctx)
	if diskRate.Read != nil || diskRate.Write != nil {
		snap.System.DiskIO = &diskRate
	}
	if netRate.Rx != nil || netRate.Tx != nil {
		snap.System.NetIO = &netRate
	}

	if a.gpu != nil {
		g := a.gpu.Read(ctx)
		snap.System.GPU = &g
	}

	snap.Processes = a.processes.Collect(ctx)

	// Build the pid list from the processes that made it into the snapshot.
	pids := make([]int32, len(snap.Processes))
	for i, row := range snap.Processes {
		pids[i] = row.PID
	}
	ioRates := a.procDiskIO.Collect(ctx, pids)
	for i, row := range snap.Processes {
		if rate, ok := ioRates[row.PID]; ok {
			row.DiskIO = &rate
			snap.Processes[i] = row
		}
	}

	snap.Docker = a.docker.Collect(ctx)

	return snap
}

// Snapshot returns the most recently assembled snapshot. It satisfies the
// server's SnapshotProvider interface, allowing the SSE hub to call it on
// every broadcast tick.
func (a *Assembler) Snapshot() model.Snapshot {
	return a.Collect(context.Background())
}
