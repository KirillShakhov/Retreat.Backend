package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"retreat-backend/internal/database"
	"sync"
	"syscall"

	"retreat-backend/internal/torrent"
	"retreat-backend/internal/utils"
)

type Server struct {
	mu             sync.Mutex
	srv            *http.Server
	url            string
	stopChan       chan os.Signal
	config         *Config
	userStore      *database.UserStore
	torrentManager *torrent.TorrentManager
	mongodb        *database.MongoDB
}

type Response struct {
	Message  string   `json:"message,omitempty"`
	Ids      []string `json:"ids,omitempty"`
	Torrents []string `json:"torrents,omitempty"`
	Files    []string `json:"files,omitempty"`
	Token    string   `json:"token,omitempty"`
}

func CreateServer(config *Config) *Server {
	port := config.Port

	server := Server{
		srv:            &http.Server{Addr: ":" + fmt.Sprint(port)},
		stopChan:       make(chan os.Signal, 1),
		config:         config,
		torrentManager: torrent.NewTorrentManager(config.Filetypes, config.DownloadPath),
	}

	signal.Notify(server.stopChan, os.Interrupt, syscall.SIGTERM)

	os.RemoveAll(config.DownloadPath)
	err := os.MkdirAll(config.DownloadPath, os.ModePerm)
	utils.Expect(err, "Failed to create downloads directory")

	mongodb, err := database.NewMongoDB(config.mongoConfig)
	utils.Expect(err, "Failed to connect to MongoDB")
	server.mongodb = mongodb

	// Initialize user store
	us := database.NewUserStore(mongodb)
	server.userStore = us

	// Public auth endpoints
	http.HandleFunc("/api/register", server.cors(server.register))
	http.HandleFunc("/api/login", server.cors(server.login))
	http.HandleFunc("/api/me", server.cors(server.auth(server.me)))

	// Protected endpoints
	http.HandleFunc("/api/play", server.cors(server.play))
	http.HandleFunc("/api/download", server.cors(server.download))
	http.HandleFunc("/api/torrents", server.cors(server.torrents))
	http.HandleFunc("/api/delete", server.cors(server.delete))

	// Keep stream public to allow external player access without token
	http.HandleFunc("/api/stream", server.cors(server.stream))
	http.HandleFunc("/api/magnet", server.cors(server.magnet))
	http.HandleFunc("/api/file", server.cors(server.file))
	http.HandleFunc("/api/list", server.cors(server.list))

	return &server
}

func (server *Server) respond(w http.ResponseWriter, res Response, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	log.Printf("%s: %s", http.StatusText(code), res.Message)
}

func (server *Server) start() (int, error) {
	listener, err := net.Listen("tcp", server.srv.Addr)
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

func (server *Server) Serve(config *Config) {
	port, err := server.start()
	utils.Expect(err, "Failed to start server")

	url := fmt.Sprintf("http://localhost:%d", port)
	log.Printf("Server running at %s/\n", url)

	server.url = url

	<-server.stopChan

	utils.Expect(server.srv.Close(), "Error closing server")
	server.torrentManager.Close()

	if err := server.mongodb.Close(); err != nil {
		log.Printf("Error closing MongoDB connection: %v", err)
	}

	err = os.RemoveAll(config.DownloadPath)
	if err != nil {
		return
	}
	err = os.MkdirAll(config.DownloadPath, os.ModePerm)
	if err != nil {
		return
	}
}
