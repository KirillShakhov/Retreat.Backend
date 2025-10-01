package server

import (
	"log"
	"net/http"
	"strings"
)

type MagnetResponse struct {
	Message string `json:"message,omitempty"`
}

func (server *Server) magnet(w http.ResponseWriter, r *http.Request) {
	uri := r.URL.Query().Get("uri")
	if uri == "" {
		server.respond(w, MagnetResponse{Message: "Missing URI"}, http.StatusBadRequest)
		return
	}

	var err error
	email, _ := r.Context().Value(userEmailKey).(string)

	user, err := server.userStore.GetUserByEmail(email)
	if err != nil {
		server.respond(w, MagnetResponse{Message: "unauthorized"}, http.StatusUnauthorized)
		return
	}

	if !strings.HasPrefix(uri, "magnet:") {
		server.respond(w, MagnetResponse{Message: "Unsupported URI format"}, http.StatusBadRequest)
		return
	}

	torrentInfo, err := server.torrentManager.AddMagnet(uri)

	if err != nil {
		server.respond(w, MagnetResponse{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
		return
	}

	if torrentInfo == nil {
		server.respond(w, MagnetResponse{Message: "Error adding torrent"}, http.StatusBadRequest)
		return
	}

	log.Printf("Loading torrent info...")

	err = server.torrentStore.CreateTorrent(user.ID, torrentInfo, uri, true)
	if err != nil {
		server.respond(w, MagnetResponse{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
		return
	}

	server.respond(w, MagnetResponse{Message: "Files added"}, http.StatusOK)
}
