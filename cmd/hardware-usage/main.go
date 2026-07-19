// Command hardware-usage serves the live resource-usage web page for this desktop.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"hardware-usage/internal/assembler"
	"hardware-usage/internal/docker"
	"hardware-usage/internal/gpu"
	"hardware-usage/internal/metrics"
	"hardware-usage/internal/server"
)

// dockerRefreshInterval is how often container stats are re-sampled in the
// background. It matches the SSE broadcast cadence so the Docker figures stay
// roughly as fresh as the rest of the snapshot without hammering the daemon.
const dockerRefreshInterval = 2 * time.Second

func main() {
	// Build the real assembler backed by production sources. Each source is
	// wrapped behind a thin project-owned interface so tests can inject fakes.
	provider, err := newAssembler()
	if err != nil {
		log.Fatal(err)
	}

	// Keep Docker stats warm off the request path so the SSE snapshot never
	// blocks on the daemon; without this the first frame waits seconds for every
	// container's stats to be sampled.
	go provider.RunDockerRefresh(context.Background(), dockerRefreshInterval)

	// Loopback by default: this is a local tool for one machine, not a network service.
	addr := defaultAddr()
	log.Printf("hardware-usage listening on http://%s", addr)
	if err := http.ListenAndServe(addr, server.NewMux(provider)); err != nil {
		log.Fatal(err)
	}
}

func newAssembler() (*assembler.Assembler, error) {
	dockerSrc, err := docker.NewDockerSource()
	if err != nil {
		return nil, err
	}
	return assembler.New(
		metrics.GopsutilSystemSource{},
		metrics.GopsutilIOSource{},
		metrics.GopsutilProcessSource{},
		metrics.NewProcFSIOSource("/proc"),
		gpu.NewReader(),
		dockerSrc,
	), nil
}

func defaultAddr() string {
	addr := os.Getenv("HW_ADDR")
	if addr == "" {
		addr = server.DefaultAddr()
	}
	return addr
}
