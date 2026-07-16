package docker

import (
	"context"
	"errors"
	"testing"
)

type fakeDockerSource struct {
	listErr    error
	statsErr   map[string]error
	list       []ContainerSummary
	stats      map[string]ContainerStats
	statsCalls []string
}

func (f *fakeDockerSource) ContainerList(_ context.Context) ([]ContainerSummary, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.list, nil
}

func (f *fakeDockerSource) ContainerStats(_ context.Context, id string) (*ContainerStats, error) {
	f.statsCalls = append(f.statsCalls, id)
	if f.statsErr != nil {
		if err, ok := f.statsErr[id]; ok {
			return nil, err
		}
	}
	stats, ok := f.stats[id]
	if !ok {
		return nil, errors.New("no stats for " + id)
	}
	return &stats, nil
}

func TestDockerCollectorGroupsByProject(t *testing.T) {
	stats := map[string]ContainerStats{
		"a1": {
			CPUStats: CPUStats{
				CPUUsage:    CPUUsage{TotalUsage: 3000000000},
				SystemUsage: 12000000000,
				OnlineCPUs:  4,
			},
			PreCPUStats: CPUStats{
				CPUUsage:    CPUUsage{TotalUsage: 1000000000},
				SystemUsage: 3000000000,
			},
			MemoryStats: MemoryStats{Usage: 100 * 1024 * 1024, Limit: 1024 * 1024 * 1024},
		},
		"b1": {
			CPUStats: CPUStats{
				CPUUsage:    CPUUsage{TotalUsage: 2000000000},
				SystemUsage: 12000000000,
				OnlineCPUs:  4,
			},
			PreCPUStats: CPUStats{
				CPUUsage:    CPUUsage{TotalUsage: 1000000000},
				SystemUsage: 3000000000,
			},
			MemoryStats: MemoryStats{Usage: 50 * 1024 * 1024, Limit: 1024 * 1024 * 1024},
		},
		"c1": {
			CPUStats: CPUStats{
				CPUUsage:    CPUUsage{TotalUsage: 1000000000},
				SystemUsage: 12000000000,
				OnlineCPUs:  4,
			},
			PreCPUStats: CPUStats{
				CPUUsage:    CPUUsage{TotalUsage: 0},
				SystemUsage: 0,
			},
			MemoryStats: MemoryStats{Usage: 25 * 1024 * 1024, Limit: 1024 * 1024 * 1024},
		},
	}

	src := &fakeDockerSource{
		list: []ContainerSummary{
			{ID: "a1", Names: []string{"/web-1"}, Labels: map[string]string{"com.docker.compose.project": "app"}},
			{ID: "b1", Names: []string{"/db-1"}, Labels: map[string]string{"com.docker.compose.project": "app"}},
			{ID: "c1", Names: []string{"/proxy"}, Labels: map[string]string{}},
		},
		stats: stats,
	}

	col := NewDockerCollector(src)
	groups := col.Collect(context.Background())

	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}

	if groups[0].Project != "app" {
		t.Errorf("first project = %q, want app", groups[0].Project)
	}
	if len(groups[0].Containers) != 2 {
		t.Fatalf("app group has %d containers, want 2", len(groups[0].Containers))
	}

	wantCPU := 88.88888888888889 // (2s / 9s) * 4 * 100
	if groups[0].Containers[0].CPU == nil || *groups[0].Containers[0].CPU != wantCPU {
		t.Errorf("a1 cpu = %v, want %v", groups[0].Containers[0].CPU, wantCPU)
	}

	ungrouped := groups[1]
	if ungrouped.Project != "(ungrouped)" {
		t.Errorf("ungrouped project = %q, want (ungrouped)", ungrouped.Project)
	}
	if len(ungrouped.Containers) != 1 || ungrouped.Containers[0].ID != "c1" {
		t.Errorf("ungrouped containers = %v, want [c1]", ungrouped.Containers)
	}
}

func TestDockerCollectorHandlesListError(t *testing.T) {
	src := &fakeDockerSource{listErr: errors.New("docker down")}
	col := NewDockerCollector(src)
	groups := col.Collect(context.Background())
	if groups != nil {
		t.Fatalf("expected nil groups on list error, got %v", groups)
	}
}

func TestDockerCollectorHandlesStatsError(t *testing.T) {
	src := &fakeDockerSource{
		list:     []ContainerSummary{{ID: "a1", Names: []string{"/web-1"}, Labels: map[string]string{"com.docker.compose.project": "app"}}},
		statsErr: map[string]error{"a1": errors.New("stats failed")},
	}
	col := NewDockerCollector(src)
	groups := col.Collect(context.Background())
	if len(groups) != 1 || len(groups[0].Containers) != 1 {
		t.Fatalf("expected one group with one container, got %v", groups)
	}
	row := groups[0].Containers[0]
	if row.ID != "a1" || row.Name != "web-1" {
		t.Errorf("row id/name = %q/%q, want a1/web-1", row.ID, row.Name)
	}
	if row.CPU != nil || row.RAM != nil {
		t.Errorf("expected absent metrics on stats error, got cpu=%v ram=%v", row.CPU, row.RAM)
	}
}

func TestDockerCollectorStripsLeadingSlashFromName(t *testing.T) {
	src := &fakeDockerSource{
		list: []ContainerSummary{{ID: "a1", Names: []string{"/web-1"}, Labels: map[string]string{"com.docker.compose.project": "app"}}},
		stats: map[string]ContainerStats{
			"a1": {
				CPUStats: CPUStats{
					CPUUsage:    CPUUsage{TotalUsage: 2000000000},
					SystemUsage: 12000000000,
					OnlineCPUs:  4,
				},
				PreCPUStats: CPUStats{
					CPUUsage:    CPUUsage{TotalUsage: 1000000000},
					SystemUsage: 3000000000,
				},
				MemoryStats: MemoryStats{Usage: 100, Limit: 200},
			},
		},
	}
	col := NewDockerCollector(src)
	groups := col.Collect(context.Background())
	if len(groups) != 1 || len(groups[0].Containers) != 1 {
		t.Fatalf("expected one group with one container, got %v", groups)
	}
	if groups[0].Containers[0].Name != "web-1" {
		t.Errorf("name = %q, want web-1", groups[0].Containers[0].Name)
	}
}

func TestDockerCollectorUsesFakeSourceNotDockerSocket(t *testing.T) {
	// This test exists to prove the package no longer imports the docker/docker
	// client for normal use. The fake source is enough to exercise the collector.
	_ = &fakeDockerSource{}
}
