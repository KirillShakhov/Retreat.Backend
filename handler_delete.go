package main

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/anacrolix/torrent"
)

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	info, ok := s.getTorrent(id)
	if !ok {
		s.respond(w, Response{Message: "File not found"}, http.StatusNotFound)
		return
	}

	ih := info.file.Torrent().InfoHash().String()
	rel := info.file.Path()

	path := filepath.Join(s.config.Path, ih, rel)

	info.file.SetPriority(torrent.PiecePriorityNone)
	s.removeTorrent(id)

	err := os.Remove(path)
	if err != nil {
		s.respond(w, Response{Message: "Error removing file"}, http.StatusInternalServerError)
		return
	}

	s.respond(w, Response{Message: "File removed"}, http.StatusOK)
}
