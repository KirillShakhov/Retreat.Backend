package server

import (
	"net/http"
)

func (server *Server) download(w http.ResponseWriter, r *http.Request) {
	//id := r.URL.Query().Get("id")

	//info, ok := server.torrentManager.GetTorrent(id)
	//if !ok {
	//	server.respond(w, Response{Message: "File not found"}, http.StatusNotFound)
	//	return
	//}

	//if info.File.Priority() == torrent.PiecePriorityNone {
	//	info.File.Download()
	//	info.Download = true
	//	server.respond(w, Response{Message: "Downloading file"}, http.StatusOK)
	//} else {
	//	info.File.SetPriority(torrent.PiecePriorityNone)
	//	info.Download = false
	//	server.respond(w, Response{Message: "File download paused"}, http.StatusOK)
	//}
}
