package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/anacrolix/torrent"
)

func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		s.respond(w, Response{Message: "Failed to parse multipart form"}, http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		s.respond(w, Response{Message: "Failed to get file from form"}, http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !strings.HasSuffix(handler.Filename, ".torrent") {
		s.respond(w, Response{Message: "Invalid file type"}, http.StatusBadRequest)
		return
	}

	tempFile, err := os.CreateTemp(s.config.Path, "upload-*.torrent")
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

	t, err := s.client.AddTorrentFromFile(tempFile.Name())
	if err != nil {
		s.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
		return
	}

	log.Printf("Loading torrent info...")

	<-t.GotInfo()

	ids := make([]string, 0)
	anyValid := false
	for _, f := range t.Files() {
		info, exists := s.getTorrent(s.getId(f))
		if !exists {
			info = s.addTorrent(f)
		}
		ids = append(ids, info.Id)
		anyValid = true
		// download first and last pieces first to start streaming asap (in theory)
		t.Piece(f.EndPieceIndex() - 1).SetPriority(torrent.PiecePriorityNow)
		t.Piece(f.BeginPieceIndex()).SetPriority(torrent.PiecePriorityNow)
	}

	if !anyValid {
		t.Drop()
		s.respond(w, Response{Message: "No valid files"}, http.StatusBadRequest)
	} else {
		s.respond(w, Response{Message: "Files added", Ids: ids}, http.StatusOK)
	}
}
