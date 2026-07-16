package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/sensors"
)

// fakeSystemSource is a deterministic SystemSource for tests.
type fakeSystemSource struct {
	cpuPct     []float64
	cpuErr     error
	vm         *mem.VirtualMemoryStat
	vmErr      error
	temps      []sensors.TemperatureStat
	tempsErr   error
	calledCPU  bool
	calledVM   bool
	calledSens bool
}

func (f *fakeSystemSource) CPUPercent(ctx context.Context) ([]float64, error) {
	f.calledCPU = true
	return f.cpuPct, f.cpuErr
}

func (f *fakeSystemSource) VirtualMemory(ctx context.Context) (*mem.VirtualMemoryStat, error) {
	f.calledVM = true
	return f.vm, f.vmErr
}

func (f *fakeSystemSource) SensorsTemperatures(ctx context.Context) ([]sensors.TemperatureStat, error) {
	f.calledSens = true
	return f.temps, f.tempsErr
}

func TestSystemCollector_FullyPopulated(t *testing.T) {
	used := uint64(8_000_000_000)
	total := uint64(16_000_000_000)
	src := &fakeSystemSource{
		cpuPct: []float64{23.5},
		vm: &mem.VirtualMemoryStat{
			Used:  used,
			Total: total,
		},
		temps: []sensors.TemperatureStat{
			{SensorKey: "coretemp_package_id0", Temperature: 55.5},
			{SensorKey: "acpitz", Temperature: 42.0},
		},
	}

	c := NewSystemCollector(src)
	snap := c.Collect(context.Background())

	if !src.calledCPU || !src.calledVM || !src.calledSens {
		t.Fatalf("expected all source methods to be called")
	}

	if snap.CPU == nil {
		t.Fatalf("expected CPU to be present")
	}
	if got := *snap.CPU; got != 23.5 {
		t.Errorf("CPU = %v, want 23.5", got)
	}

	if snap.RAM == nil {
		t.Fatalf("expected RAM to be present")
	}
	if snap.RAM.Used == nil || *snap.RAM.Used != used {
		t.Errorf("RAM.Used = %v, want %v", snap.RAM.Used, used)
	}
	if snap.RAM.Total == nil || *snap.RAM.Total != total {
		t.Errorf("RAM.Total = %v, want %v", snap.RAM.Total, total)
	}

	if len(snap.Temps) != 2 {
		t.Fatalf("expected 2 temperatures, got %d", len(snap.Temps))
	}
	wantLabels := []string{"coretemp_package_id0", "acpitz"}
	wantVals := []float64{55.5, 42.0}
	for i, got := range snap.Temps {
		if got.Label != wantLabels[i] {
			t.Errorf("Temp[%d].Label = %q, want %q", i, got.Label, wantLabels[i])
		}
		if got.Value == nil || *got.Value != wantVals[i] {
			t.Errorf("Temp[%d].Value = %v, want %v", i, got.Value, wantVals[i])
		}
	}
}

func TestSystemCollector_MissingTemperatures_Absent(t *testing.T) {
	src := &fakeSystemSource{
		cpuPct:   []float64{5.0},
		vm:       &mem.VirtualMemoryStat{Used: 1, Total: 2},
		tempsErr: errors.New("sensors not available"),
	}

	c := NewSystemCollector(src)
	snap := c.Collect(context.Background())

	if snap.CPU == nil || snap.RAM == nil {
		t.Fatalf("expected CPU and RAM to be present")
	}
	if len(snap.Temps) != 0 {
		t.Errorf("expected temperatures to be absent/empty when source errors, got %v", snap.Temps)
	}
}

func TestSystemCollector_EmptyTemperatures_Absent(t *testing.T) {
	src := &fakeSystemSource{
		cpuPct: []float64{5.0},
		vm:     &mem.VirtualMemoryStat{Used: 1, Total: 2},
		temps:  []sensors.TemperatureStat{},
	}

	c := NewSystemCollector(src)
	snap := c.Collect(context.Background())

	if len(snap.Temps) != 0 {
		t.Errorf("expected empty temperature slice when source returns none, got %v", snap.Temps)
	}
}

func TestSystemCollector_CPUErrors_Absent(t *testing.T) {
	src := &fakeSystemSource{
		cpuErr: errors.New("cpu busy"),
		vm:     &mem.VirtualMemoryStat{Used: 1, Total: 2},
	}

	c := NewSystemCollector(src)
	snap := c.Collect(context.Background())

	if snap.CPU != nil {
		t.Errorf("expected CPU to be absent on error, got %v", *snap.CPU)
	}
	if snap.RAM == nil {
		t.Fatalf("expected RAM to still be present")
	}
}

func TestSystemCollector_VMErrors_Absent(t *testing.T) {
	src := &fakeSystemSource{
		cpuPct: []float64{12.0},
		vmErr:  errors.New("/proc/meminfo denied"),
	}

	c := NewSystemCollector(src)
	snap := c.Collect(context.Background())

	if snap.RAM != nil {
		t.Errorf("expected RAM to be absent on error, got %v", snap.RAM)
	}
	if snap.CPU == nil || *snap.CPU != 12.0 {
		t.Errorf("expected CPU to still be present with value 12.0")
	}
}

func TestGopsutilSystemSource_ImplementsInterface(t *testing.T) {
	// This is a compile-time assertion that the production source implements
	// the project-owned interface.
	var _ SystemSource = GopsutilSystemSource{}
}

// Ensure the fake source also satisfies the interface (compile check).
var _ SystemSource = (*fakeSystemSource)(nil)
