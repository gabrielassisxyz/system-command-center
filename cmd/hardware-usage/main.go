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
	addr := os.Getenv("HW_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8765"
	}
	log.Printf("hardware-usage listening on http://%s", addr)
	if err := http.ListenAndServe(addr, server.NewMux()); err != nil {
		log.Fatal(err)
	}
}
