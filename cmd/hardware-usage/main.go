// Command hardware-usage serves the live resource-usage web page for this desktop.
package main

import (
	"log"
	"net/http"
	"os"

	"hardware-usage/internal/server"
)

func main() {
	// Loopback by default: this is a local tool for one machine, not a network service.
	addr := defaultAddr()
	log.Printf("hardware-usage listening on http://%s", addr)
	if err := http.ListenAndServe(addr, server.NewMux(server.EmptyProvider{})); err != nil {
		log.Fatal(err)
	}
}

func defaultAddr() string {
	addr := os.Getenv("HW_ADDR")
	if addr == "" {
		addr = server.DefaultAddr()
	}
	return addr
}
