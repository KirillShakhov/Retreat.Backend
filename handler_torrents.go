package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func (s *Server) torrents(w http.ResponseWriter, r *http.Request) {
	torrents := s.getTorrents()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(torrents); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	log.Printf("%s: %s", http.StatusText(http.StatusOK), torrents)
}
