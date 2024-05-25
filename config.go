package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Port      int      `json:"port"`
	Filetypes []string `json:"filetypes"`
	Playback  []string `json:"playback"`
	Path      string   `json:"path"`
	file      string
}

func loadConfig() (*Config, error) {
	config := Config{
		Port:      8000,
		Filetypes: []string{".mkv", ".mp4"},
		Playback:  []string{"mpv", "--no-terminal", "--force-window", "--ytdl-format=best"},
		Path:      filepath.Join(exPath, "downloads"),
		file:      filepath.Join(exPath, "config.json"),
	}

	err := os.MkdirAll(config.Path, os.ModePerm)
	expect(err, "Failed to create downloads directory")

	err = loadJSON(config.file, &config)
	if err != nil {
		if os.IsNotExist(err) {
			config.save()
			return &config, nil // It's fine if the file doesn't exist
		}
	}
	return &config, err
}

func (c *Config) save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.file, data, 0644)
}
