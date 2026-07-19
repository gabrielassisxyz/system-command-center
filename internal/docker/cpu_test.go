package docker

import "testing"

func TestCalculateContainerCPU(t *testing.T) {
	cpu := calculateContainerCPU(nil)
	if cpu != nil {
		t.Errorf("nil stats should return nil cpu, got %v", cpu)
	}

	// Zero pre-read stats should yield nil CPU.
	cpu = calculateContainerCPU(&ContainerStats{
		CPUStats:    CPUStats{CPUUsage: CPUUsage{TotalUsage: 100}, SystemUsage: 1000, OnlineCPUs: 4},
		PreCPUStats: CPUStats{},
	})
	if cpu != nil {
		t.Errorf("zero pre-read should yield nil cpu, got %v", cpu)
	}

	cpu = calculateContainerCPU(&ContainerStats{
		CPUStats: CPUStats{
			CPUUsage:    CPUUsage{TotalUsage: 3000000000},
			SystemUsage: 12000000000,
			OnlineCPUs:  4,
		},
		PreCPUStats: CPUStats{
			CPUUsage:    CPUUsage{TotalUsage: 1000000000},
			SystemUsage: 3000000000,
		},
	})
	if cpu == nil {
		t.Fatal("expected cpu value")
	}
	want := 88.88888888888889
	if *cpu != want {
		t.Errorf("cpu = %v, want %v", *cpu, want)
	}
}
