package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
)

type TorrentInfo struct {
	Id       string    `json:"id"`
	Name     string    `json:"name"`
	Progress int       `json:"progress"`
	Download bool      `json:"download"`
	Time     time.Time `json:"time"`
	file     *torrent.File
}

type Server struct {
	mu          sync.Mutex
	srv         *http.Server
	url         string
	stopChan    chan os.Signal
	client      *torrent.Client
	torrentInfo map[string]*TorrentInfo
	config      *Config
	userStore   *UserStore
}

type Response struct {
	Message  string         `json:"message,omitempty"`
	Ids      []string       `json:"ids,omitempty"`
	Torrents []*TorrentInfo `json:"torrents,omitempty"`
	Files    []string       `json:"files,omitempty"`
	Token    string         `json:"token,omitempty"`
}

func createServer(config *Config) *Server {
	port := config.Port
	server := Server{
		srv:         &http.Server{Addr: ":" + fmt.Sprint(port)},
		stopChan:    make(chan os.Signal, 1),
		torrentInfo: make(map[string]*TorrentInfo),
		config:      config,
	}

	signal.Notify(server.stopChan, os.Interrupt, syscall.SIGTERM)

	os.RemoveAll(config.Path)
	err := os.MkdirAll(config.Path, os.ModePerm)
	expect(err, "Failed to create downloads directory")

	cfg := torrent.NewDefaultClientConfig()
	cfg.DefaultStorage = storage.NewFileByInfoHash(config.Path)

	cfg.EstablishedConnsPerTorrent = 55
	cfg.HalfOpenConnsPerTorrent = 30

	client, err := torrent.NewClient(cfg)
	expect(err, "Failed to create torrent client")
	server.client = client

	// Initialize user store
	us, err := NewUserStore(config.UsersFile)
	expect(err, "Failed to initialize user store")
	server.userStore = us

	// Public auth endpoints
	http.HandleFunc("/api/register", server.cors(server.register))
	http.HandleFunc("/api/login", server.cors(server.login))
	http.HandleFunc("/api/me", server.cors(server.auth(server.me)))

	// Protected endpoints
	http.HandleFunc("/api/play", server.cors(server.auth(server.play)))
	http.HandleFunc("/api/download", server.cors(server.auth(server.download)))
	http.HandleFunc("/api/torrents", server.cors(server.auth(server.torrents)))
	http.HandleFunc("/api/delete", server.cors(server.auth(server.delete)))

	// Keep stream public to allow external player access without token
	http.HandleFunc("/api/stream", server.cors(server.stream))
	http.HandleFunc("/api/magnet", server.cors(server.auth(server.magnet)))
	http.HandleFunc("/api/file", server.cors(server.auth(server.file)))
	http.HandleFunc("/api/list", server.cors(server.auth(server.list)))

	return &server
}

func (s *Server) respond(w http.ResponseWriter, res Response, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	log.Printf("%s: %s", http.StatusText(code), res.Message)
}

func (s *Server) start() (int, error) {
	listener, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return 0, err
	}

	go func() {
		if err := http.Serve(listener, nil); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	port := listener.Addr().(*net.TCPAddr).Port

	return port, nil
}

func (s *Server) getId(f *torrent.File) string {
	id := f.Torrent().InfoHash().String() + f.DisplayPath()
	return fmt.Sprintf("%x", md5.Sum([]byte(id)))
}

func (s *Server) addTorrent(f *torrent.File) *TorrentInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.getId(f)

	info := TorrentInfo{
		Id:   id,
		Name: f.DisplayPath(),
		Time: time.Now(),
		file: f,
	}

	s.torrentInfo[id] = &info
	return &info
}

func (s *Server) getTorrent(id string) (*TorrentInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.torrentInfo[id]
	return t, ok
}

func (s *Server) getTorrents() []*TorrentInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	torrents := make([]*TorrentInfo, 0, len(s.torrentInfo))
	for _, info := range s.torrentInfo {
		info.Progress = int(info.file.BytesCompleted() * 100 / info.file.Length())
		torrents = append(torrents, info)
	}

	sort.Slice(torrents, func(i, j int) bool {
		return torrents[i].Time.After(torrents[j].Time)
	})

	return torrents
}

func (s *Server) removeTorrent(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.torrentInfo, id)
}

func (s *Server) isValidFile(f *torrent.File) bool {
	ext := path.Ext(f.Path())
	return slices.Contains(s.config.Filetypes, ext)
}

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

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	info, ok := s.getTorrent(id)
	if !ok {
		s.respond(w, Response{Message: "File not found"}, http.StatusNotFound)
		return
	}

	ih := info.file.Torrent().InfoHash().String()
	rel := info.file.Path()

	path := filepath.Join(s.config.Path, ih, rel)

	info.file.SetPriority(torrent.PiecePriorityNone)
	s.removeTorrent(id)

	err := os.Remove(path)
	if err != nil {
		s.respond(w, Response{Message: "Error removing file"}, http.StatusInternalServerError)
		return
	}

	s.respond(w, Response{Message: "File removed"}, http.StatusOK)
}

func (s *Server) play(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	_, ok := s.getTorrent(id)
	if !ok {
		s.respond(w, Response{Message: "File not found"}, http.StatusNotFound)
		return
	}

	args := append(s.config.Playback[1:], server.url+"/stream?f="+id)
	cmd := exec.Command(s.config.Playback[0], args...)
	if err := cmd.Start(); err != nil {
		s.respond(w, Response{Message: "Error starting playback"}, http.StatusInternalServerError)
	} else {
		s.respond(w, Response{Message: "Playback started"}, http.StatusOK)
	}
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	info, ok := s.getTorrent(id)
	if !ok {
		s.respond(w, Response{Message: "File not found"}, http.StatusNotFound)
		return
	}

	if info.file.Priority() == torrent.PiecePriorityNone {
		info.file.Download()
		info.Download = true
		s.respond(w, Response{Message: "Downloading file"}, http.StatusOK)
	} else {
		info.file.SetPriority(torrent.PiecePriorityNone)
		info.Download = false
		s.respond(w, Response{Message: "File download paused"}, http.StatusOK)
	}
}

func (s *Server) torrents(w http.ResponseWriter, r *http.Request) {
	torrents := s.getTorrents()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(torrents); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	log.Printf("%s: %s", http.StatusText(http.StatusOK), torrents)
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	info, ok := s.getTorrent(id)
	if !ok {
		s.respond(w, Response{Message: "File not found"}, http.StatusNotFound)
		return
	}

	fn := info.file.DisplayPath()

	w.Header().Set("Expires", "0")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
	w.Header().Set("Content-Disposition", "attachment; filename="+fn)
	w.Header().Set("Access-Control-Allow-Origin", "*")

	reader := info.file.NewReader()
	reader.SetReadahead(info.file.Length() / 100)
	reader.SetResponsive()

	http.ServeContent(w, r, fn, time.Now(), reader)
}

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

func serve(config *Config) {
	server = createServer(config)

	port, err := server.start()
	expect(err, "Failed to start server")

	url := fmt.Sprintf("http://localhost:%d", port)
	log.Printf("Server running at %s/\n", url)

	server.url = url

	<-server.stopChan

	expect(server.srv.Close(), "Error closing server")
	server.client.Close()

	err = os.RemoveAll(config.Path)
	if err != nil {
		return
	}
	err = os.MkdirAll(config.Path, os.ModePerm)
	if err != nil {
		return
	}
}
