package server

import (
	"log"
	"net/http"
	"strings"
)

func (server *Server) magnet(w http.ResponseWriter, r *http.Request) {
	uri := r.URL.Query().Get("uri")
	if uri == "" {
		server.respond(w, Response{Message: "Missing URI"}, http.StatusBadRequest)
		return
	}

	var files []string
	var err error
	switch {
	case strings.HasPrefix(uri, "magnet:"):
		files, err = server.torrentManager.AddMagnet(uri)
	default:
		server.respond(w, Response{Message: "Unsupported URI format"}, http.StatusBadRequest)
		return
	}
	if err != nil {
		server.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
		return
	}

	log.Printf("Loading torrent info...")

	if len(files) > 0 {
		server.respond(w, Response{Message: "Files added", Ids: files}, http.StatusOK)
	} else {
		server.respond(w, Response{Message: "No valid files"}, http.StatusBadRequest)
	}
}
