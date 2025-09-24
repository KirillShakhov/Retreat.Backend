package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/anacrolix/torrent"
)

func (s *Server) magnet(w http.ResponseWriter, r *http.Request) {
	uri := r.URL.Query().Get("uri")
	if uri == "" {
		s.respond(w, Response{Message: "Missing URI"}, http.StatusBadRequest)
		return
	}

	var t *torrent.Torrent
	var err error
	switch {
	case strings.HasPrefix(uri, "magnet:"):
		t, err = s.client.AddMagnet(uri)
	default:
		s.respond(w, Response{Message: "Unsupported URI format"}, http.StatusBadRequest)
		return
	}
	if err != nil {
		s.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
		return
	}

	log.Printf("Loading torrent info...")

	<-t.GotInfo()

	ids := make([]string, 0)
	anyValid := false
	for _, f := range t.Files() {
		if s.isValidFile(f) {
			info, exists := s.getTorrent(s.getId(f))
			if !exists {
				info = s.addTorrent(f)
			}
			ids = append(ids, info.Id)
			anyValid = true
			// download first and last pieces first to start streaming asap (in theory)
			t.Piece(f.EndPieceIndex() - 1).SetPriority(torrent.PiecePriorityNow)
			t.Piece(f.BeginPieceIndex()).SetPriority(torrent.PiecePriorityNow)
		} else {
			f.SetPriority(torrent.PiecePriorityNone)
		}
	}

	if anyValid {
		s.respond(w, Response{Message: "Files added", Ids: ids}, http.StatusOK)
	} else {
		t.Drop()
		s.respond(w, Response{Message: "No valid files"}, http.StatusBadRequest)
	}
}
