package server

import (
	"net/http"
	"os/exec"
)

func (server *Server) play(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	_, ok := server.torrentManager.GetTorrent(id)
	if !ok {
		server.respond(w, Response{Message: "File not found"}, http.StatusNotFound)
		return
	}

	args := append(server.config.Playback[1:], server.url+"/stream?f="+id)
	cmd := exec.Command(server.config.Playback[0], args...)
	if err := cmd.Start(); err != nil {
		server.respond(w, Response{Message: "Error starting playback"}, http.StatusInternalServerError)
	} else {
		server.respond(w, Response{Message: "Playback started"}, http.StatusOK)
	}
}
