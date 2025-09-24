package server

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/anacrolix/torrent"
)

func (server *Server) delete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	info, ok := server.torrentClient.GetTorrent(id)
	if !ok {
		server.respond(w, Response{Message: "File not found"}, http.StatusNotFound)
		return
	}

	ih := info.File.Torrent().InfoHash().String()
	rel := info.File.Path()

	path := filepath.Join(server.config.Path, ih, rel)

	info.File.SetPriority(torrent.PiecePriorityNone)
	server.torrentClient.RemoveTorrent(id)

	err := os.Remove(path)
	if err != nil {
		server.respond(w, Response{Message: "Error removing file"}, http.StatusInternalServerError)
		return
	}

	server.respond(w, Response{Message: "File removed"}, http.StatusOK)
}
