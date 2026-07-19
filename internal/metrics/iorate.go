// Package metrics collects system and process resource metrics behind thin,
// project-owned interfaces so tests can inject fakes.
package metrics

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/net"

	"hardware-usage/internal/model"
)

// IOClock abstracts time for rate calculations so tests can control the
// interval between two Collect() calls deterministically.
type IOClock interface {
	Now() time.Time
}

// systemClock is the production IOClock backed by time.Now().
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// IOSource abstracts the cumulative disk and network I/O counters needed to
// compute whole-machine read/write and rx/tx rates. Tests inject a fake.
type IOSource interface {
	DiskIOCounters(ctx context.Context) (map[string]disk.IOCountersStat, error)
	NetIOCounters(ctx context.Context) ([]net.IOCountersStat, error)
}

// GopsutilIOSource is the production IOSource backed by gopsutil.
type GopsutilIOSource struct{}

// DiskIOCounters returns per-block-device cumulative I/O counters.
func (GopsutilIOSource) DiskIOCounters(ctx context.Context) (map[string]disk.IOCountersStat, error) {
	return disk.IOCountersWithContext(ctx)
}

// NetIOCounters returns per-interface cumulative I/O counters.
func (GopsutilIOSource) NetIOCounters(ctx context.Context) ([]net.IOCountersStat, error) {
	return net.IOCountersWithContext(ctx, false)
}

// ioSample holds a single point-in-time reading of all cumulative counters.
type ioSample struct {
	time time.Time
	disk map[string]disk.IOCountersStat
	net  []net.IOCountersStat
}

// IORateCollector turns cumulative disk and network counters into byte-rate
// snapshots. It is stateful: the first Collect() call stores a baseline and
// returns zero rates; subsequent calls return delta / deltaT.
type IORateCollector struct {
	src   IOSource
	clock IOClock
	prev  *ioSample
}

// NewIORateCollector creates a collector backed by src and the real clock.
func NewIORateCollector(src IOSource) *IORateCollector {
	return NewIORateCollectorWithClock(src, systemClock{})
}

// NewIORateCollectorWithClock creates a collector with an explicit clock.
func NewIORateCollectorWithClock(src IOSource, clock IOClock) *IORateCollector {
	return &IORateCollector{src: src, clock: clock}
}

// Collect returns whole-machine disk and network I/O rates. The first call
// always returns zero rates because there is no previous sample to diff against.
// Source errors leave the corresponding rate field absent.
func (c *IORateCollector) Collect(ctx context.Context) (model.DiskIORate, model.NetIORate) {
	now := c.clock.Now()

	diskCounters, diskErr := c.src.DiskIOCounters(ctx)
	netCounters, netErr := c.src.NetIOCounters(ctx)

	// Copy the source data so a caller that mutates returned maps/slices
	// cannot corrupt the previous sample we diff against next time.
	diskCopy := make(map[string]disk.IOCountersStat, len(diskCounters))
	for name, stat := range diskCounters {
		diskCopy[name] = stat
	}
	netCopy := make([]net.IOCountersStat, len(netCounters))
	copy(netCopy, netCounters)

	sample := &ioSample{time: now, disk: diskCopy, net: netCopy}

	var diskRate model.DiskIORate
	var netRate model.NetIORate

	if c.prev != nil {
		dt := sample.time.Sub(c.prev.time).Seconds()
		if dt > 0 {
			if diskErr == nil {
				rd, wr := diffDisk(c.prev.disk, sample.disk)
				rdRate := rd / dt
				wrRate := wr / dt
				diskRate.Read = &rdRate
				diskRate.Write = &wrRate
			}
			if netErr == nil {
				rx, tx := diffNet(c.prev.net, sample.net)
				rxRate := rx / dt
				txRate := tx / dt
				netRate.Rx = &rxRate
				netRate.Tx = &txRate
			}
		}
	}

	c.prev = sample
	return diskRate, netRate
}

func diffDisk(prev, cur map[string]disk.IOCountersStat) (readBytes, writeBytes float64) {
	for name, curStat := range cur {
		prevStat, ok := prev[name]
		if !ok {
			continue
		}
		if curStat.ReadBytes >= prevStat.ReadBytes {
			readBytes += float64(curStat.ReadBytes - prevStat.ReadBytes)
		}
		if curStat.WriteBytes >= prevStat.WriteBytes {
			writeBytes += float64(curStat.WriteBytes - prevStat.WriteBytes)
		}
	}
	return readBytes, writeBytes
}

func diffNet(prev, cur []net.IOCountersStat) (rxBytes, txBytes float64) {
	for _, curStat := range cur {
		prevStat := findNetByName(prev, curStat.Name)
		if prevStat == nil {
			continue
		}
		if curStat.BytesRecv >= prevStat.BytesRecv {
			rxBytes += float64(curStat.BytesRecv - prevStat.BytesRecv)
		}
		if curStat.BytesSent >= prevStat.BytesSent {
			txBytes += float64(curStat.BytesSent - prevStat.BytesSent)
		}
	}
	return rxBytes, txBytes
}

func findNetByName(stats []net.IOCountersStat, name string) *net.IOCountersStat {
	for i := range stats {
		if stats[i].Name == name {
			return &stats[i]
		}
	}
	return nil
}
