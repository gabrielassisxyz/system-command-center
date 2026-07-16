// Package docker collects Docker container metrics and groups them by compose
// project label, behind a thin project-owned interface so tests never need a
// running daemon.
//
// This file contains the minimal Docker Engine API structs needed by the
// collector. Defining them here lets the project avoid importing the
// github.com/docker/docker module, which currently has open govulncheck
// advisories for APIs we do not use (archive copy, plugin privileges, etc.).
// The collector only needs two endpoints: GET /containers/json and GET
// /containers/{id}/stats?stream=false.
package docker

import "time"

// containerSummary is the subset of the Docker Engine API "containers/json"
// response that this package uses.
// ContainerSummary is the subset of the Docker Engine API "containers/json"
// response that this package uses. It is exported so tests in other packages
// can build fake DockerSource implementations.
type ContainerSummary struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Labels map[string]string `json:"Labels"`
}

// ContainerStats is the subset of the Docker Engine API "containers/{id}/stats"
// response that this package uses. It is exported so tests in other packages
// can build fake DockerSource implementations.
type ContainerStats struct {
	Read        time.Time   `json:"read"`
	PreRead     time.Time   `json:"preread"`
	CPUStats    CPUStats    `json:"cpu_stats"`
	PreCPUStats CPUStats    `json:"precpu_stats"`
	MemoryStats MemoryStats `json:"memory_stats"`
}

// CPUUsage is the subset of Docker CPU usage statistics used by this package.
type CPUUsage struct {
	TotalUsage  uint64   `json:"total_usage"`
	PercpuUsage []uint64 `json:"percpu_usage,omitempty"`
}

// CPUStats is the subset of Docker CPU statistics used by this package.
type CPUStats struct {
	CPUUsage    CPUUsage `json:"cpu_usage"`
	SystemUsage uint64   `json:"system_cpu_usage,omitempty"`
	OnlineCPUs  uint32   `json:"online_cpus,omitempty"`
}

// MemoryStats is the subset of Docker memory statistics used by this package.
type MemoryStats struct {
	Usage uint64 `json:"usage,omitempty"`
	Limit uint64 `json:"limit,omitempty"`
}
