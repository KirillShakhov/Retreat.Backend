package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"retreat-backend/utils"
)

type Config struct {
	Port          int      `json:"port"`
	Filetypes     []string `json:"filetypes"`
	Playback      []string `json:"playback"`
	Path          string   `json:"path"`
	JWTSecret     string   `json:"jwt_secret"`
	UsersFile     string   `json:"users_file"`
	TokenTTLHours int      `json:"token_ttl_hours"`
	file          string
}

func LoadConfig() (*Config, error) {
	config := Config{
		Port:          8000,
		Filetypes:     []string{".mkv", ".mp4"},
		Playback:      []string{"mpv", "--no-terminal", "--force-window", "--ytdl-format=best"},
		Path:          filepath.Join("downloads"),
		JWTSecret:     "",
		UsersFile:     filepath.Join("data/users.json"),
		TokenTTLHours: 24,
		file:          filepath.Join("data/config.json"),
	}

	err := os.MkdirAll(config.Path, os.ModePerm)
	utils.Expect(err, "Failed to create downloads directory")

	_ = os.MkdirAll(filepath.Dir(config.file), os.ModePerm)

	err = utils.LoadJSON(config.file, &config)
	if err != nil {
		if os.IsNotExist(err) {
			if config.JWTSecret == "" {
				config.JWTSecret = generateRandomHex(32)
			}
			config.save()
			return &config, nil // It's fine if the file doesn't exist
		}
	}
	// Ensure secret exists even if config file was present but missing it
	if config.JWTSecret == "" {
		config.JWTSecret = generateRandomHex(32)
		_ = config.save()
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

func generateRandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fallback static; better than empty
		return "change-me-" + hex.EncodeToString([]byte("fallback-secret"))
	}
	return hex.EncodeToString(b)
}
