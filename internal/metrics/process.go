package metrics

import (
	"context"

	"github.com/shirou/gopsutil/v4/process"

	"hardware-usage/internal/model"
)

// ProcessInfo groups the per-process readings needed to build a ProcessRow.
type ProcessInfo struct {
	PID  int32
	Name string
	CPU  float64
	RAM  process.MemoryInfoStat
}

// ProcessSource abstracts gopsutil process enumeration and per-process metric
// reads so tests can inject fakes.
type ProcessSource interface {
	Processes(ctx context.Context) ([]*process.Process, error)
	Info(ctx context.Context, p *process.Process) (ProcessInfo, error)
}

// GopsutilProcessSource is the production ProcessSource backed by gopsutil.
type GopsutilProcessSource struct{}

// Processes returns all running processes.
func (GopsutilProcessSource) Processes(ctx context.Context) ([]*process.Process, error) {
	return process.ProcessesWithContext(ctx)
}

// Info reads name, CPU percent, and memory info for a single process. If any
// individual metric errors, the returned ProcessInfo still carries what we
// could read; the collector decides how to represent missing values.
func (GopsutilProcessSource) Info(ctx context.Context, p *process.Process) (ProcessInfo, error) {
	info := ProcessInfo{PID: p.Pid}

	if name, err := p.NameWithContext(ctx); err == nil {
		info.Name = name
	}

	if cpu, err := p.CPUPercentWithContext(ctx); err == nil {
		info.CPU = cpu
	}

	if mem, err := p.MemoryInfoWithContext(ctx); err == nil && mem != nil {
		info.RAM = *mem
	}

	return info, nil
}

// ProcessCollector builds a list of model.ProcessRow from a ProcessSource. A
// process whose metric reads fail is skipped rather than aborting the whole
// list; its individual fields are omitted when absent.
type ProcessCollector struct {
	src ProcessSource
}

// NewProcessCollector creates a collector backed by src.
func NewProcessCollector(src ProcessSource) *ProcessCollector {
	return &ProcessCollector{src: src}
}

// Collect returns per-process rows. Errors reading individual processes are
// silently ignored; the rest of the list is still returned.
func (c *ProcessCollector) Collect(ctx context.Context) []model.ProcessRow {
	procs, err := c.src.Processes(ctx)
	if err != nil {
		return nil
	}

	rows := make([]model.ProcessRow, 0, len(procs))
	for _, p := range procs {
		info, err := c.src.Info(ctx, p)
		if err != nil {
			continue
		}

		// A process with no name is not useful in the UI.
		if info.Name == "" {
			continue
		}

		row := model.ProcessRow{
			PID:  info.PID,
			Name: info.Name,
		}

		cpu := info.CPU
		row.CPU = &cpu

		if info.RAM.RSS > 0 {
			rss := info.RAM.RSS
			row.RAM = &model.RAMInfo{Used: &rss}
		}

		rows = append(rows, row)
	}

	return rows
}
