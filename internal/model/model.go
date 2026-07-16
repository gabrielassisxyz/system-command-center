// Package model defines the backend↔frontend snapshot contract.
//
// All metrics that may be missing on a given machine are represented as pointers
// so that a nil value, combined with `omitempty`, produces either a JSON null or an
// omitted key. The frontend treats missing metrics as absent rather than zero.
package model

// RAMInfo carries used/total byte counts for a process, container, or the whole
// system. Either or both fields may be absent.
type RAMInfo struct {
	Used  *uint64 `json:"used,omitempty"`
	Total *uint64 `json:"total,omitempty"`
}

// DiskIORate is a diffed read/write rate in bytes per second.
type DiskIORate struct {
	Read  *float64 `json:"read,omitempty"`
	Write *float64 `json:"write,omitempty"`
}

// NetIORate is a diffed receive/transmit rate in bytes per second.
type NetIORate struct {
	Rx *float64 `json:"rx,omitempty"`
	Tx *float64 `json:"tx,omitempty"`
}

// TemperatureSensor is one kernel-reported thermal zone.
type TemperatureSensor struct {
	Label string   `json:"label"`
	Value *float64 `json:"value,omitempty"`
}

// GPUInfo holds AMD GPU metrics read from sysfs. Every field is optional because
// the files may be missing or the hardware may not expose them.
type GPUInfo struct {
	Busy      *float64 `json:"busy,omitempty"`
	VRAMUsed  *uint64  `json:"vram_used,omitempty"`
	VRAMTotal *uint64  `json:"vram_total,omitempty"`
	Temp      *float64 `json:"temp,omitempty"`
}

// SystemSnapshot is the whole-machine header broadcast with every frame.
type SystemSnapshot struct {
	CPU    *float64            `json:"cpu,omitempty"`
	RAM    *RAMInfo            `json:"ram,omitempty"`
	DiskIO *DiskIORate         `json:"disk_io,omitempty"`
	NetIO  *NetIORate          `json:"net_io,omitempty"`
	Temps  []TemperatureSensor `json:"temps,omitempty"`
	GPU    *GPUInfo            `json:"gpu,omitempty"`
}

// ProcessRow is one entry in the per-process list.
type ProcessRow struct {
	PID    int32       `json:"pid"`
	Name   string      `json:"name"`
	CPU    *float64    `json:"cpu,omitempty"`
	RAM    *RAMInfo    `json:"ram,omitempty"`
	DiskIO *DiskIORate `json:"disk_io,omitempty"`
}

// ContainerRow is one Docker container inside a compose group.
type ContainerRow struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	CPU  *float64 `json:"cpu,omitempty"`
	RAM  *RAMInfo `json:"ram,omitempty"`
}

// ComposeGroup groups containers by the com.docker.compose.project label.
// Containers with no label are placed under the project name "(ungrouped)".
type ComposeGroup struct {
	Project    string         `json:"project"`
	Containers []ContainerRow `json:"containers"`
}

// Snapshot is the full frame pushed over SSE.
type Snapshot struct {
	System    SystemSnapshot `json:"system"`
	Processes []ProcessRow   `json:"processes"`
	Docker    []ComposeGroup `json:"docker,omitempty"`
}
