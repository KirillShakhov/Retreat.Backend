package main

import (
	"log"
)

var (
	ex     string
	exPath string
	server *Server
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime)

	config, err := loadConfig()
	expect(err, "Failed to load config")

	serve(config)
}
