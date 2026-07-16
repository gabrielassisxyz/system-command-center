// Package metrics collects system and process resource metrics behind thin,
// project-owned interfaces so tests can inject fakes.
package metrics

import (
	"context"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/sensors"

	"hardware-usage/internal/model"
)

// SystemSource abstracts the gopsutil calls needed for whole-machine CPU,
// memory, and temperature readings. Tests inject a fake implementation instead
// of touching real hardware.
type SystemSource interface {
	CPUPercent(ctx context.Context) ([]float64, error)
	VirtualMemory(ctx context.Context) (*mem.VirtualMemoryStat, error)
	SensorsTemperatures(ctx context.Context) ([]sensors.TemperatureStat, error)
}

// GopsutilSystemSource is the production SystemSource backed by gopsutil.
type GopsutilSystemSource struct{}

// CPUPercent returns the system-wide CPU usage percentage. An interval of 0
// asks gopsutil to compare against its internally cached previous sample.
func (GopsutilSystemSource) CPUPercent(ctx context.Context) ([]float64, error) {
	return cpu.PercentWithContext(ctx, 0, false)
}

// VirtualMemory returns RAM statistics from the kernel.
func (GopsutilSystemSource) VirtualMemory(ctx context.Context) (*mem.VirtualMemoryStat, error) {
	return mem.VirtualMemoryWithContext(ctx)
}

// SensorsTemperatures returns temperature readings reported by the kernel.
func (GopsutilSystemSource) SensorsTemperatures(ctx context.Context) ([]sensors.TemperatureStat, error) {
	return sensors.TemperaturesWithContext(ctx)
}

// SystemCollector builds a model.SystemSnapshot from a SystemSource. It is
// absent-tolerant: a source error for any individual metric leaves that metric
// out of the snapshot rather than failing the whole collection.
type SystemCollector struct {
	src SystemSource
}

// NewSystemCollector creates a collector backed by src.
func NewSystemCollector(src SystemSource) *SystemCollector {
	return &SystemCollector{src: src}
}

// Collect returns a SystemSnapshot populated from the source. Missing metrics
// are represented as nil pointer fields or an empty temperature slice.
func (c *SystemCollector) Collect(ctx context.Context) model.SystemSnapshot {
	var snap model.SystemSnapshot

	if pcts, err := c.src.CPUPercent(ctx); err == nil && len(pcts) > 0 {
		// Copy the value so a caller that reuses the slice cannot mutate
		// the pointer we store in the snapshot.
		cpu := pcts[0]
		snap.CPU = &cpu
	}

	if vm, err := c.src.VirtualMemory(ctx); err == nil && vm != nil {
		snap.RAM = &model.RAMInfo{
			Used:  &vm.Used,
			Total: &vm.Total,
		}
	}

	if temps, err := c.src.SensorsTemperatures(ctx); err == nil {
		for _, t := range temps {
			v := t.Temperature
			snap.Temps = append(snap.Temps, model.TemperatureSensor{
				Label: t.SensorKey,
				Value: &v,
			})
		}
	}

	return snap
}
