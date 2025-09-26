package server

import (
	"net/http"
)

func (server *Server) stream(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	fileId := r.URL.Query().Get("fileId")

	info, ok := server.torrentManager.Stream(w, r, id, fileId)
	if !ok {
		server.respond(w, Response{Message: info}, http.StatusNotFound)
		return
	}
}
