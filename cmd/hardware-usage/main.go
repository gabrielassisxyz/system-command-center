// Command hardware-usage serves the live resource-usage web page for this desktop.
package main

import (
	"log"
	"net/http"
	"os"

	"hardware-usage/internal/assembler"
	"hardware-usage/internal/docker"
	"hardware-usage/internal/gpu"
	"hardware-usage/internal/metrics"
	"hardware-usage/internal/server"
)

func main() {
	// Build the real assembler backed by production sources. Each source is
	// wrapped behind a thin project-owned interface so tests can inject fakes.
	provider, err := newAssembler()
	if err != nil {
		log.Fatal(err)
	}

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
