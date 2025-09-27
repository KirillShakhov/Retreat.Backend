package server

import (
	"encoding/json"
	"log"
	"net/http"
	"retreat-backend/internal/torrent"
)

func (server *Server) torrents(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(userEmailKey).(string)

	user, err := server.userStore.GetUserByEmail(email)
	if err != nil {
		log.Println(err)
		return
	}

	torrents, err := server.torrentStore.GetTorrents(user.ID)
	if err != nil {
		log.Println(err)
		return
	}
	var torrentInfos []*torrent.TorrentInfo
	torrentInfos = make([]*torrent.TorrentInfo, 0, len(torrents))
	for _, t := range torrents {
		torrentInfos = append(torrentInfos, t.TorrentInfo)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(torrentInfos); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	log.Printf("%s: %s", http.StatusText(http.StatusOK), torrentInfos)
}
