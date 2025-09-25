package server

import (
	"encoding/json"
	"log"
	"net/http"
)

func (server *Server) torrents(w http.ResponseWriter, r *http.Request) {
	torrents := server.torrentManager.GetTorrents()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(torrents); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	log.Printf("%s: %s", http.StatusText(http.StatusOK), torrents)
}
