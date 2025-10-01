package server

import (
	"net/http"
)

type DeleteResponse struct {
	Message string `json:"message,omitempty"`
}

func (server *Server) delete(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(userEmailKey).(string)
	user, err := server.userStore.GetUserByEmail(email)
	if err != nil {
		return
	}

	id := r.URL.Query().Get("id")

	err = server.torrentStore.DeleteTorrent(user.ID, id)

	isHave := server.torrentStore.HaveTorrent(id)
	if !isHave {
		server.torrentManager.RemoveTorrent(id)
	}

	if err != nil {
		server.respond(w, DeleteResponse{Message: "torrent not found"}, http.StatusNotFound)
		return
	}
}
