package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
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
