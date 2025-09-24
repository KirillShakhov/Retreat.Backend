package server

import (
	"github.com/anacrolix/torrent"
	"io/ioutil"
	"log"
	"net/http"
)

func (server *Server) list(w http.ResponseWriter, r *http.Request) {
	var t *torrent.Torrent
	var err error

	uri := r.URL.Query().Get("uri")
	file, _, _ := r.FormFile("file")

	if uri != "" {
		t, err = server.client.AddMagnet(uri)
		if err != nil {
			server.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
			return
		}
	} else if file != nil {
		tempFile, err := ioutil.TempFile(server.config.Path, "upload-*.torrent")
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

		t, err = server.client.AddTorrentFromFile(tempFile.Name())
		if err != nil {
			server.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
			return
		}
	} else {
		server.respond(w, Response{Message: "No URI or file provided"}, http.StatusBadRequest)
		return
	}

	log.Printf("Loading torrent info...")

	<-t.GotInfo()

	files := make([]string, 0)
	for _, f := range t.Files() {
		files = append(files, f.DisplayPath())
	}

	server.respond(w, Response{Files: files}, http.StatusOK)
}
