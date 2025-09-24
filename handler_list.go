package main

import (
	"io/ioutil"
	"log"
	"net/http"

	"github.com/anacrolix/torrent"
)

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	var t *torrent.Torrent
	var err error

	uri := r.URL.Query().Get("uri")
	file, _, _ := r.FormFile("file")

	if uri != "" {
		t, err = s.client.AddMagnet(uri)
		if err != nil {
			s.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
			return
		}
	} else if file != nil {
		tempFile, err := ioutil.TempFile(s.config.Path, "upload-*.torrent")
		if err != nil {
			s.respond(w, Response{Message: "Failed to create temp file"}, http.StatusInternalServerError)
			return
		}
		defer tempFile.Close()

		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			s.respond(w, Response{Message: "Failed to read file"}, http.StatusInternalServerError)
			return
		}

		if _, err = tempFile.Write(fileBytes); err != nil {
			s.respond(w, Response{Message: "Failed to write to temp file"}, http.StatusInternalServerError)
			return
		}

		t, err = s.client.AddTorrentFromFile(tempFile.Name())
		if err != nil {
			s.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
			return
		}
	} else {
		s.respond(w, Response{Message: "No URI or file provided"}, http.StatusBadRequest)
		return
	}

	log.Printf("Loading torrent info...")

	<-t.GotInfo()

	files := make([]string, 0)
	for _, f := range t.Files() {
		files = append(files, f.DisplayPath())
	}

	s.respond(w, Response{Files: files}, http.StatusOK)
}
