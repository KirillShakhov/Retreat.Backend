package server

import (
	"log"
	"net/http"
	"retreat-backend/internal/torrent"
	"strings"
)

func (server *Server) magnet(w http.ResponseWriter, r *http.Request) {
	uri := r.URL.Query().Get("uri")
	if uri == "" {
		server.respond(w, Response{Message: "Missing URI"}, http.StatusBadRequest)
		return
	}

	var err error
	email, _ := r.Context().Value(userEmailKey).(string)

	user, err := server.userStore.GetUserByEmail(email)
	if err != nil {
		server.respond(w, Response{Message: "unauthorized"}, http.StatusUnauthorized)
		return
	}

	var torrentInfo *torrent.TorrentInfo
	switch {
	case strings.HasPrefix(uri, "magnet:"):
		torrentInfo, err = server.torrentManager.AddMagnet(uri)
	default:
		server.respond(w, Response{Message: "Unsupported URI format"}, http.StatusBadRequest)
		return
	}

	if err != nil {
		server.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
		return
	}

	if torrentInfo == nil {
		server.respond(w, Response{Message: "Error adding torrent"}, http.StatusBadRequest)
		return
	}

	log.Printf("Loading torrent info...")

	err = server.torrentStore.CreateTorrent(user.ID, torrentInfo, uri, true)
	if err != nil {
		server.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
		return
	}

	server.respond(w, Response{Message: "Files added"}, http.StatusOK)
}
