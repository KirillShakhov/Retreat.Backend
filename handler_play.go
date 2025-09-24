package main

import (
	"net/http"
	"os/exec"
)

func (s *Server) play(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	_, ok := s.getTorrent(id)
	if !ok {
		s.respond(w, Response{Message: "File not found"}, http.StatusNotFound)
		return
	}

	args := append(s.config.Playback[1:], server.url+"/stream?f="+id)
	cmd := exec.Command(s.config.Playback[0], args...)
	if err := cmd.Start(); err != nil {
		s.respond(w, Response{Message: "Error starting playback"}, http.StatusInternalServerError)
	} else {
		s.respond(w, Response{Message: "Playback started"}, http.StatusOK)
	}
}
