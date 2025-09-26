package torrent

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// TorrentManager управляет загрузкой и обработкой торрент-файлов
type TorrentManager struct {
	mu           sync.Mutex
	filetypes    []string
	client       *torrent.Client
	downloadPath string
}

type FileInfo struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Progress int    `json:"progress"`
}

// TorrentInfo содержит информацию о загружаемом файле
type TorrentInfo struct {
	Id    string      `json:"id"`
	Name  string      `json:"name"`
	Time  time.Time   `json:"time"`
	Files []*FileInfo `json:"files"`
}

// NewTorrentManager создает новый менеджер торрентов
func NewTorrentManager(filetypes []string, downloadPath string) *TorrentManager {
	cfg := torrent.NewDefaultClientConfig()
	cfg.DefaultStorage = storage.NewFileByInfoHash(downloadPath)
	cfg.EstablishedConnsPerTorrent = 55
	cfg.HalfOpenConnsPerTorrent = 30

	client, err := torrent.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create torrent client: %v", err)
	}

	return &TorrentManager{
		filetypes:    filetypes,
		client:       client,
		downloadPath: downloadPath,
	}
}

// AddTorrentFromFile добавляет торрент из файла
func (tm *TorrentManager) AddTorrentFromFile(torrentFile io.Reader, filename string) (*TorrentInfo, error) {
	// Создаем временный файл для торрента
	tempFile, err := os.CreateTemp(tm.downloadPath, "upload-*.torrent")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Копируем содержимое торрент-файла во временный файл
	if _, err := io.Copy(tempFile, torrentFile); err != nil {
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}

	// Добавляем торрент в клиент
	t, err := tm.client.AddTorrentFromFile(tempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("error adding torrent: %w", err)
	}

	log.Printf("Loading torrent info for %s...", filename)

	// Ждем получения информации о торренте
	<-t.GotInfo()

	// Обрабатываем файлы торрента
	_, err = tm.processTorrentFiles(t)
	if err != nil {
		t.Drop()
		return nil, err
	}

	torrentInfo := convertTorrent(t)

	return torrentInfo, nil
}

// processTorrentFiles обрабатывает файлы в добавленном торренте
func (tm *TorrentManager) processTorrentFiles(t *torrent.Torrent) (bool, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	anyValid := false
	for _, f := range t.Files() {
		if !tm.isValidFile(f) {
			continue
		}

		anyValid = true

		// Устанавливаем приоритеты для быстрого старта потоковой передачи
		tm.setStreamingPriorities(f)
	}

	if !anyValid {
		return false, fmt.Errorf("no valid files in torrent %s", t.Name())
	}

	return true, nil
}

// generateFileID генерирует уникальный ID для файла
func generateFileID(f *torrent.File) string {
	id := f.Torrent().InfoHash().String() + f.DisplayPath()
	return fmt.Sprintf("%x", md5.Sum([]byte(id)))
}

// setStreamingPriorities устанавливает приоритеты для потоковой передачи
func (tm *TorrentManager) setStreamingPriorities(f *torrent.File) {
	t := f.Torrent()

	// Скачиваем первый и последний куски для быстрого старта потоковой передачи
	if f.BeginPieceIndex() < f.EndPieceIndex() {
		t.Piece(f.EndPieceIndex() - 1).SetPriority(torrent.PiecePriorityNow)
		t.Piece(f.BeginPieceIndex()).SetPriority(torrent.PiecePriorityNow)
	}
}

// isValidFile проверяет, является ли файл допустимым для обработки
func (tm *TorrentManager) isValidFile(f *torrent.File) bool {
	ext := strings.ToLower(path.Ext(f.Path()))
	return slices.Contains(tm.filetypes, ext)
}

// GetTorrent возвращает информацию о торренте по ID
func (tm *TorrentManager) GetTorrent(id string) (*TorrentInfo, bool) {
	var hash metainfo.Hash
	err := hash.FromHexString(id)
	if err != nil {
		return nil, false
	}

	t, ok := tm.client.Torrent(hash)
	return convertTorrent(t), ok
}

func (tm *TorrentManager) Stream(w http.ResponseWriter, r *http.Request, id string, fileId string) (string, bool) {
	var hash metainfo.Hash
	err := hash.FromHexString(id)
	if err != nil {
		return "hash is not valid", false
	}

	t, ok := tm.client.Torrent(hash)
	if !ok {
		return "file not found", false
	}

	for _, file := range t.Files() {
		if generateFileID(file) == fileId {
			fn := file.DisplayPath()

			w.Header().Set("Expires", "0")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
			w.Header().Set("Content-Disposition", "attachment; filename="+fn)
			w.Header().Set("Access-Control-Allow-Origin", "*")

			reader := file.NewReader()
			reader.SetReadahead(file.Length() / 100)
			reader.SetResponsive()

			http.ServeContent(w, r, fn, time.Now(), reader)

			return "fie found", true
		}
	}

	return "file not found", false
}

// GetTorrents возвращает список всех торрентов
func (tm *TorrentManager) GetTorrents() []*TorrentInfo {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	torrents := convertTorrentList(tm.client.Torrents())

	// Сортируем по времени добавления (новые сначала)
	sort.Slice(torrents, func(i, j int) bool {
		return torrents[i].Time.After(torrents[j].Time)
	})

	return torrents
}

// calculateProgress вычисляет прогресс загрузки файла
func calculateProgress(file *torrent.File) int {
	if file == nil || file.Length() == 0 {
		return 0
	}
	return int(file.BytesCompleted() * 100 / file.Length())
}

func (tm *TorrentManager) RemoveTorrent(id string) (bool, string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	var hash metainfo.Hash
	err := hash.FromHexString(id)
	if err != nil {
		return false, "hash is not valid"
	}

	t, ok := tm.client.Torrent(hash)
	if !ok {
		return true, "torrent not found"
	}

	for _, file := range t.Files() {
		file.SetPriority(torrent.PiecePriorityNone)

		ih := file.Torrent().InfoHash().String()
		rel := file.Path()
		filePath := filepath.Join(tm.downloadPath, ih, rel)

		_ = os.Remove(filePath)
	}

	return true, "torrent removed"
}

// GetFilepath возвращает путь к загруженному файлу
func (tm *TorrentManager) GetFilepath(id string, fileId string) (string, error) {
	var hash metainfo.Hash
	err := hash.FromHexString(id)
	if err != nil {
		return "", fmt.Errorf("hash is not valid: %s", id)
	}

	t, exists := tm.client.Torrent(hash)
	if !exists {
		return "", fmt.Errorf("torrent not found: %s", id)
	}

	for _, file := range t.Files() {
		if generateFileID(file) == fileId {
			torrentPath := filepath.Join(tm.downloadPath, t.InfoHash().String())
			filename := filepath.Base(file.Path())

			return filepath.Join(torrentPath, filename), nil
		}
	}

	return "", fmt.Errorf("file not found: %s", fileId)
}

// Close закрывает торрент-клиент
func (tm *TorrentManager) Close() {
	if tm.client != nil {
		tm.client.Close()
	}
}

func (tm *TorrentManager) AddMagnet(uri string) (bool, error) {
	t, err := tm.client.AddMagnet(uri)
	if err != nil {
		return false, err
	}

	<-t.GotInfo()

	isValid, err := tm.processTorrentFiles(t)
	if err != nil {
		t.Drop()
	}

	return isValid, err
}

func (tm *TorrentManager) getId(f *torrent.File) string {
	id := f.Torrent().InfoHash().String() + f.DisplayPath()
	return fmt.Sprintf("%x", md5.Sum([]byte(id)))
}

func convertTorrentList(torrentList []*torrent.Torrent) []*TorrentInfo {
	torrents := make([]*TorrentInfo, 0, len(torrentList))
	for _, t := range torrentList {
		torrents = append(torrents, convertTorrent(t))
	}

	return torrents
}

func convertTorrent(t *torrent.Torrent) *TorrentInfo {
	files := t.Files()

	fileInfos := make([]*FileInfo, 0, len(files))
	for _, f := range files {
		fileInfo := &FileInfo{
			Id:       generateFileID(f),
			Name:     f.DisplayPath(),
			Progress: calculateProgress(f),
		}

		fileInfos = append(fileInfos, fileInfo)
	}

	torrentInfo := &TorrentInfo{
		Id:    t.InfoHash().String(),
		Name:  t.Name(),
		Time:  time.Unix(0, t.Metainfo().CreationDate),
		Files: fileInfos,
	}

	return torrentInfo
}
