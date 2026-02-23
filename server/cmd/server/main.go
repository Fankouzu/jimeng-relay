package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	if err := runServer(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func runServer() error {
	// Basic config check to satisfy TDD startup failure test
	ak := os.Getenv("VOLC_ACCESSKEY")
	sk := os.Getenv("VOLC_SECRETKEY")

	if ak == "" || sk == "" {
		return fmt.Errorf("missing required configuration: VOLC_ACCESSKEY and VOLC_SECRETKEY must be set")
	}

	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting jimeng-relay server on port %s...", port)

	// Placeholder for actual server startup
	// In a real scenario, this would block. For now, we just return nil if config is present.
	// This will be expanded in future tasks.
	return nil
}
