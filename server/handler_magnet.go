package server

import (
	"log"
	"net/http"
	"strings"

	"github.com/anacrolix/torrent"
)

func (server *Server) magnet(w http.ResponseWriter, r *http.Request) {
	uri := r.URL.Query().Get("uri")
	if uri == "" {
		server.respond(w, Response{Message: "Missing URI"}, http.StatusBadRequest)
		return
	}

	var t *torrent.Torrent
	var err error
	switch {
	case strings.HasPrefix(uri, "magnet:"):
		t, err = server.client.AddMagnet(uri)
	default:
		server.respond(w, Response{Message: "Unsupported URI format"}, http.StatusBadRequest)
		return
	}
	if err != nil {
		server.respond(w, Response{Message: "Error adding torrent: " + err.Error()}, http.StatusBadRequest)
		return
	}

	log.Printf("Loading torrent info...")

	<-t.GotInfo()

	ids := make([]string, 0)
	anyValid := false
	for _, f := range t.Files() {
		if server.torrentClient.IsValidFile(f) {
			info, exists := server.torrentClient.GetTorrent(server.torrentClient.GetId(f))
			if !exists {
				info = server.torrentClient.AddTorrent(f)
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
		server.respond(w, Response{Message: "Files added", Ids: ids}, http.StatusOK)
	} else {
		t.Drop()
		server.respond(w, Response{Message: "No valid files"}, http.StatusBadRequest)
	}
}
