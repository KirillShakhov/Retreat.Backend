package main

import (
	"net/http"

	"github.com/anacrolix/torrent"
)

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	info, ok := s.getTorrent(id)
	if !ok {
		s.respond(w, Response{Message: "File not found"}, http.StatusNotFound)
		return
	}

	if info.file.Priority() == torrent.PiecePriorityNone {
		info.file.Download()
		info.Download = true
		s.respond(w, Response{Message: "Downloading file"}, http.StatusOK)
	} else {
		info.file.SetPriority(torrent.PiecePriorityNone)
		info.Download = false
		s.respond(w, Response{Message: "File download paused"}, http.StatusOK)
	}
}
