package main

import (
	"log"
	serverMain "retreat-backend/internal/server"
	"retreat-backend/internal/utils"
)

var (
	server *serverMain.Server
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime)

	config, err := serverMain.LoadConfig()
	utils.Expect(err, "Failed to load config")
	server = serverMain.CreateServer(config)
	server.Serve(config)
}
