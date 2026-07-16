package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/net"
)

// fakeIOSource is a deterministic IOSource for tests.
type fakeIOSource struct {
	disk    map[string]disk.IOCountersStat
	diskErr error
	net     []net.IOCountersStat
	netErr  error
}

func (f *fakeIOSource) DiskIOCounters(_ context.Context) (map[string]disk.IOCountersStat, error) {
	return f.disk, f.diskErr
}

func (f *fakeIOSource) NetIOCounters(_ context.Context) ([]net.IOCountersStat, error) {
	return f.net, f.netErr
}

// fakeClock steps forward by a fixed delta on each Now() call.
type fakeClock struct {
	now   time.Time
	delta time.Duration
}

func (f *fakeClock) Now() time.Time {
	cur := f.now
	f.now = f.now.Add(f.delta)
	return cur
}

func TestIORateCollector_FirstCallYieldsZeroRates(t *testing.T) {
	src := &fakeIOSource{
		disk: map[string]disk.IOCountersStat{
			"nvme0n1": {ReadBytes: 1000, WriteBytes: 2000},
		},
		net: []net.IOCountersStat{{Name: "eth0", BytesRecv: 3000, BytesSent: 4000}},
	}
	c := NewIORateCollector(src)
	c.clock = &fakeClock{now: time.Unix(0, 0), delta: time.Second}

	dr, nr := c.Collect(context.Background())

	if dr.Read != nil || dr.Write != nil {
		t.Errorf("first disk rates should be absent, got read=%v write=%v", dr.Read, dr.Write)
	}
	if nr.Rx != nil || nr.Tx != nil {
		t.Errorf("first net rates should be absent, got rx=%v tx=%v", nr.Rx, nr.Tx)
	}
}

func TestIORateCollector_SecondCallYieldsDiffedRates(t *testing.T) {
	src := &fakeIOSource{
		disk: map[string]disk.IOCountersStat{
			"nvme0n1": {ReadBytes: 1000, WriteBytes: 2000},
		},
		net: []net.IOCountersStat{{Name: "eth0", BytesRecv: 3000, BytesSent: 4000}},
	}
	c := NewIORateCollector(src)
	c.clock = &fakeClock{now: time.Unix(0, 0), delta: 2 * time.Second}

	c.Collect(context.Background())

	src.disk = map[string]disk.IOCountersStat{
		"nvme0n1": {ReadBytes: 3000, WriteBytes: 7000},
	}
	src.net = []net.IOCountersStat{{Name: "eth0", BytesRecv: 8000, BytesSent: 12000}}

	dr, nr := c.Collect(context.Background())

	if dr.Read == nil || *dr.Read != 1000.0 {
		t.Errorf("disk read rate = %v, want 1000", dr.Read)
	}
	if dr.Write == nil || *dr.Write != 2500.0 {
		t.Errorf("disk write rate = %v, want 2500", dr.Write)
	}
	if nr.Rx == nil || *nr.Rx != 2500.0 {
		t.Errorf("net rx rate = %v, want 2500", nr.Rx)
	}
	if nr.Tx == nil || *nr.Tx != 4000.0 {
		t.Errorf("net tx rate = %v, want 4000", nr.Tx)
	}
}

func TestIORateCollector_MissingDeviceOnFirstCall_Ignored(t *testing.T) {
	src := &fakeIOSource{
		disk: map[string]disk.IOCountersStat{
			"nvme0n1": {ReadBytes: 1000, WriteBytes: 2000},
		},
		net: []net.IOCountersStat{{Name: "eth0", BytesRecv: 3000, BytesSent: 4000}},
	}
	c := NewIORateCollector(src)
	c.clock = &fakeClock{now: time.Unix(0, 0), delta: time.Second}

	c.Collect(context.Background())

	src.disk = map[string]disk.IOCountersStat{
		"nvme0n1": {ReadBytes: 2000, WriteBytes: 3000},
		"sda":     {ReadBytes: 500, WriteBytes: 500},
	}
	src.net = []net.IOCountersStat{
		{Name: "eth0", BytesRecv: 4000, BytesSent: 5000},
		{Name: "wlan0", BytesRecv: 1000, BytesSent: 2000},
	}

	dr, nr := c.Collect(context.Background())

	if dr.Read == nil || *dr.Read != 1000.0 {
		t.Errorf("disk read rate = %v, want 1000 (new device should be ignored)", dr.Read)
	}
	if dr.Write == nil || *dr.Write != 1000.0 {
		t.Errorf("disk write rate = %v, want 1000", dr.Write)
	}
	if nr.Rx == nil || *nr.Rx != 1000.0 {
		t.Errorf("net rx rate = %v, want 1000 (new interface should be ignored)", nr.Rx)
	}
	if nr.Tx == nil || *nr.Tx != 1000.0 {
		t.Errorf("net tx rate = %v, want 1000", nr.Tx)
	}
}

func TestIORateCollector_DeviceRemoved_Dropped(t *testing.T) {
	src := &fakeIOSource{
		disk: map[string]disk.IOCountersStat{
			"nvme0n1": {ReadBytes: 1000, WriteBytes: 2000},
			"sda":     {ReadBytes: 500, WriteBytes: 500},
		},
	}
	c := NewIORateCollector(src)
	c.clock = &fakeClock{now: time.Unix(0, 0), delta: time.Second}

	c.Collect(context.Background())

	src.disk = map[string]disk.IOCountersStat{
		"nvme0n1": {ReadBytes: 2000, WriteBytes: 3000},
	}

	dr, _ := c.Collect(context.Background())

	if dr.Read == nil || *dr.Read != 1000.0 {
		t.Errorf("disk read rate = %v, want 1000 (removed device should be dropped)", dr.Read)
	}
}

func TestIORateCollector_DiskSourceError_Absent(t *testing.T) {
	src := &fakeIOSource{
		disk:    map[string]disk.IOCountersStat{"nvme0n1": {ReadBytes: 1000}},
		diskErr: errors.New("disk permission denied"),
		net:     []net.IOCountersStat{{Name: "eth0", BytesRecv: 3000, BytesSent: 4000}},
	}
	c := NewIORateCollector(src)
	c.clock = &fakeClock{now: time.Unix(0, 0), delta: time.Second}

	c.Collect(context.Background())
	src.net = []net.IOCountersStat{{Name: "eth0", BytesRecv: 6000, BytesSent: 8000}}

	dr, nr := c.Collect(context.Background())

	if dr.Read != nil || dr.Write != nil {
		t.Errorf("disk rates should be absent on source error, got read=%v write=%v", dr.Read, dr.Write)
	}
	if nr.Rx == nil || *nr.Rx != 3000.0 {
		t.Errorf("net rx rate = %v, want 3000", nr.Rx)
	}
}

func TestIORateCollector_NetSourceError_Absent(t *testing.T) {
	src := &fakeIOSource{
		disk:   map[string]disk.IOCountersStat{"nvme0n1": {ReadBytes: 1000, WriteBytes: 2000}},
		net:    []net.IOCountersStat{{Name: "eth0", BytesRecv: 3000, BytesSent: 4000}},
		netErr: errors.New("net permission denied"),
	}
	c := NewIORateCollector(src)
	c.clock = &fakeClock{now: time.Unix(0, 0), delta: time.Second}

	c.Collect(context.Background())
	src.disk = map[string]disk.IOCountersStat{"nvme0n1": {ReadBytes: 3000, WriteBytes: 5000}}

	dr, nr := c.Collect(context.Background())

	if dr.Read == nil || *dr.Read != 2000.0 {
		t.Errorf("disk read rate = %v, want 2000", dr.Read)
	}
	if nr.Rx != nil || nr.Tx != nil {
		t.Errorf("net rates should be absent on source error, got rx=%v tx=%v", nr.Rx, nr.Tx)
	}
}

func TestIORateCollector_ZeroDelta_NoDivideByZero(t *testing.T) {
	src := &fakeIOSource{
		disk: map[string]disk.IOCountersStat{"nvme0n1": {ReadBytes: 1000}},
		net:  []net.IOCountersStat{{Name: "eth0", BytesRecv: 3000}},
	}
	c := NewIORateCollector(src)
	// Clock does not advance, so delta is 0 seconds.
	c.clock = &fakeClock{now: time.Unix(0, 0), delta: 0}

	c.Collect(context.Background())
	src.disk = map[string]disk.IOCountersStat{"nvme0n1": {ReadBytes: 2000}}
	src.net = []net.IOCountersStat{{Name: "eth0", BytesRecv: 4000}}

	dr, nr := c.Collect(context.Background())

	if dr.Read != nil || dr.Write != nil {
		t.Errorf("expected zero rates on zero delta, got disk read=%v write=%v", dr.Read, dr.Write)
	}
	if nr.Rx != nil || nr.Tx != nil {
		t.Errorf("expected zero rates on zero delta, got net rx=%v tx=%v", nr.Rx, nr.Tx)
	}
}

func TestGopsutilIOSource_ImplementsInterface(t *testing.T) {
	var _ IOSource = GopsutilIOSource{}
}

var _ IOSource = (*fakeIOSource)(nil)
