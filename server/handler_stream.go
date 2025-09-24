package server

import (
	"net/http"
	"time"
)

func (server *Server) stream(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	info, ok := server.torrentClient.GetTorrent(id)
	if !ok {
		server.respond(w, Response{Message: "File not found"}, http.StatusNotFound)
		return
	}

	fn := info.File.DisplayPath()

	w.Header().Set("Expires", "0")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
	w.Header().Set("Content-Disposition", "attachment; filename="+fn)
	w.Header().Set("Access-Control-Allow-Origin", "*")

	reader := info.File.NewReader()
	reader.SetReadahead(info.File.Length() / 100)
	reader.SetResponsive()

	http.ServeContent(w, r, fn, time.Now(), reader)
}
