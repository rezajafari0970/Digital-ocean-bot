package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:18080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "digital-ocean-bot-api"})
	})
	log.Printf("api listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
