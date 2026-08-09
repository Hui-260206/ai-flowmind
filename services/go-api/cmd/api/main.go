package main

import (
	"errors"
	"log"
	"net/http"

	"ai-flowmind/services/go-api/internal/config"
	"ai-flowmind/services/go-api/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}

	httpServer := server.New()
	httpServer.Addr = cfg.HTTPAddr

	log.Printf("go-api listening on %s", cfg.HTTPAddr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("start HTTP server on %s: %v", cfg.HTTPAddr, err)
	}
}
