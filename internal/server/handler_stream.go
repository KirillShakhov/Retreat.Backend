package server

import (
	"net/http"
)

func (server *Server) stream(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(userEmailKey).(string)
	user, err := server.userStore.GetUserByEmail(email)
	if err != nil {
		return
	}

	id := r.URL.Query().Get("id")
	fileId := r.URL.Query().Get("fileId")

	torrent, err := server.torrentStore.GetTorrent(user.ID, id)
	if err != nil {
		server.respond(w, Response{Message: "torrent not found"}, http.StatusNotFound)
		return
	}

	_, isHave := server.torrentManager.GetTorrent(torrent.Hash)
	if !isHave {
		if torrent.IsMagnet {
			_, err := server.torrentManager.AddMagnet(torrent.TorrentFile)
			if err != nil {
				server.respond(w, Response{Message: "torrent not found"}, http.StatusNotFound)
				return
			}
		} else {
			server.respond(w, Response{Message: "torrent not found"}, http.StatusNotFound)
			return
		}
	}

	info, ok := server.torrentManager.Stream(w, r, id, fileId)
	if !ok {
		server.respond(w, Response{Message: info}, http.StatusNotFound)
		return
	}
}
