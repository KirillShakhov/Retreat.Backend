package server

import (
	"net/http"
)

func (server *Server) delete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	isSuccess, message := server.torrentManager.RemoveTorrent(id)

	if !isSuccess {
		server.respond(w, Response{Message: message}, http.StatusBadRequest)
		return
	}

	server.respond(w, Response{Message: message}, http.StatusOK)
}
