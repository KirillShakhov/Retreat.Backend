package server

import (
	"io/ioutil"
	"net/http"
)

func (server *Server) list(w http.ResponseWriter, r *http.Request) {
	var files []string

	uri := r.URL.Query().Get("uri")
	file, _, _ := r.FormFile("file")

	if uri != "" {
		files, err := server.torrentManager.AddMagnet(uri)
		if err != nil {
			server.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
			return
		}

		server.respond(w, Response{Files: files}, http.StatusOK)
		return
	} else if file != nil {
		tempFile, err := ioutil.TempFile(server.config.DownloadPath, "upload-*.torrent")
		if err != nil {
			server.respond(w, Response{Message: "Failed to create temp file"}, http.StatusInternalServerError)
			return
		}
		defer tempFile.Close()

		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			server.respond(w, Response{Message: "Failed to read file"}, http.StatusInternalServerError)
			return
		}

		if _, err = tempFile.Write(fileBytes); err != nil {
			server.respond(w, Response{Message: "Failed to write to temp file"}, http.StatusInternalServerError)
			return
		}

		files, err = server.torrentManager.AddTorrentFromFile(tempFile, tempFile.Name())
		if err != nil {
			server.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
			return
		}

		server.respond(w, Response{Files: files}, http.StatusOK)
		return
	} else {
		server.respond(w, Response{Message: "No URI or file provided"}, http.StatusBadRequest)
		return
	}
}
