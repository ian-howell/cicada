package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	dataDir := flag.String("data-dir", "./data", "directory for SQLite database and log files")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("failed to create data dir: %v", err)
	}

	log.Printf("cicada starting: addr=%s data-dir=%s", *addr, *dataDir)
	// wiring will be added as components are built
}
