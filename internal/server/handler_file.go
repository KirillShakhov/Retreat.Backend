package server

import (
	"net/http"
	"strings"
)

func (server *Server) file(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB max
		server.respond(w, Response{Message: "Failed to parse multipart form"}, http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		server.respond(w, Response{Message: "Failed to get file from form"}, http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !strings.HasSuffix(handler.Filename, ".torrent") {
		server.respond(w, Response{Message: "Invalid file type"}, http.StatusBadRequest)
		return
	}

	// Используем менеджер торрентов для обработки файла
	_, err = server.torrentManager.AddTorrentFromFile(file, handler.Filename)
	if err != nil {
		server.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
		return
	}

	server.respond(w, Response{Message: "Files added"}, http.StatusOK)
}
