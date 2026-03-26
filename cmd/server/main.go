package main

import (
	"log"

	"github.com/taylor-zha/lockstep/internal/server"
)

func main() {
	s, err := server.New("config/config.yaml")
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := s.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
