package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"time"

	"hardware-usage/internal/model"
)

const (
	ungroupedProject = "(ungrouped)"
	defaultVersion   = "v1.41"
)

// DockerSource abstracts the Docker Engine HTTP API calls needed to list
// containers and fetch their one-shot stats. Tests inject a fake implementation.
type DockerSource interface {
	ContainerList(ctx context.Context) ([]ContainerSummary, error)
	ContainerStats(ctx context.Context, containerID string) (*ContainerStats, error)
}

// NewDockerSource creates a production DockerSource by talking to the local
// Docker daemon over its Unix socket. If DOCKER_HOST is set, it is parsed for
// the socket path; otherwise /var/run/docker.sock is used.
//
// The returned source returns errors from individual calls, but the collector
// treats those as empty groups. This keeps the dependency surface minimal: we
// issue plain HTTP requests to the Engine API rather than importing the heavy
// docker/docker client module, whose current releases are flagged by
// govulncheck for APIs this project never uses.
func NewDockerSource() (DockerSource, error) {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}
	addr, err := parseHost(host)
	if err != nil {
		return nil, err
	}
	return &httpSource{
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, addr.network, addr.path)
				},
			},
			Timeout: 10 * time.Second,
		},
		version: defaultVersion,
	}, nil
}

type dialAddr struct {
	network string
	path    string
}

func parseHost(host string) (dialAddr, error) {
	switch {
	case len(host) >= 7 && host[:7] == "unix://":
		return dialAddr{network: "unix", path: host[7:]}, nil
	case len(host) >= 6 && host[:6] == "tcp://":
		return dialAddr{network: "tcp", path: host[6:]}, nil
	}
	return dialAddr{}, fmt.Errorf("unsupported DOCKER_HOST: %s", host)
}

type httpSource struct {
	client  *http.Client
	version string
}

func (s *httpSource) ContainerList(ctx context.Context) ([]ContainerSummary, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://docker/"+s.version+"/containers/json", http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker list: %s", resp.Status)
	}
	var out []ContainerSummary
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *httpSource) ContainerStats(ctx context.Context, id string) (*ContainerStats, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://docker/"+s.version+"/containers/"+id+"/stats?stream=false", http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker stats %s: %s", id, resp.Status)
	}
	var out ContainerStats
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DockerCollector builds model.ComposeGroups from a DockerSource. It is
// absent-tolerant: a container whose stats cannot be read is still listed,
// but with absent metrics.
type DockerCollector struct {
	src DockerSource
}

// NewDockerCollector creates a collector backed by src.
func NewDockerCollector(src DockerSource) *DockerCollector {
	return &DockerCollector{src: src}
}

// Collect returns containers grouped by the com.docker.compose.project label.
// If the source cannot list containers, it returns nil (treated as empty by
// the assembler) rather than surfacing an error.
func (c *DockerCollector) Collect(ctx context.Context) []model.ComposeGroup {
	containers, err := c.src.ContainerList(ctx)
	if err != nil {
		return nil
	}

	groups := make(map[string][]model.ContainerRow)
	for _, ctr := range containers {
		project := ctr.Labels["com.docker.compose.project"]
		if project == "" {
			project = ungroupedProject
		}
		row := c.row(ctx, ctr)
		groups[project] = append(groups[project], row)
	}

	return sortGroups(groups)
}

func (c *DockerCollector) row(ctx context.Context, ctr ContainerSummary) model.ContainerRow {
	row := model.ContainerRow{
		ID:   ctr.ID,
		Name: firstName(ctr.Names),
	}

	stats, err := c.src.ContainerStats(ctx, ctr.ID)
	if err != nil {
		return row
	}

	row.CPU = calculateContainerCPU(stats)

	if stats.MemoryStats.Usage > 0 {
		used := stats.MemoryStats.Usage
		total := stats.MemoryStats.Limit
		row.RAM = &model.RAMInfo{Used: &used, Total: &total}
	}

	return row
}

// calculateContainerCPU turns Docker CPU deltas into a percentage. One-shot
// stats usually have zeroed PreCPUStats, so we prefer the delta path when both
// system usages are present.
func calculateContainerCPU(stats *ContainerStats) *float64 {
	if stats == nil {
		return nil
	}

	if stats.CPUStats.SystemUsage > 0 && stats.PreCPUStats.SystemUsage > 0 {
		cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
		sysDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
		if sysDelta > 0 {
			cpus := stats.CPUStats.OnlineCPUs
			if cpus == 0 {
				cpus = uint32(len(stats.CPUStats.CPUUsage.PercpuUsage))
			}
			if cpus == 0 {
				cpus = 1
			}
			cpu := (cpuDelta / sysDelta) * float64(cpus) * 100.0
			return &cpu
		}
	}
	return nil
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	name := names[0]
	if len(name) > 0 && name[0] == '/' {
		return name[1:]
	}
	return name
}

func sortGroups(groups map[string][]model.ContainerRow) []model.ComposeGroup {
	projects := make([]string, 0, len(groups))
	for p := range groups {
		projects = append(projects, p)
	}
	sort.Strings(projects)

	// Keep the ungrouped bucket last so named compose projects appear first.
	for i, p := range projects {
		if p == ungroupedProject {
			projects = append(projects[:i], projects[i+1:]...)
			projects = append(projects, p)
			break
		}
	}

	out := make([]model.ComposeGroup, 0, len(projects))
	for _, p := range projects {
		rows := groups[p]
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].ID < rows[j].ID
		})
		out = append(out, model.ComposeGroup{Project: p, Containers: rows})
	}
	return out
}
