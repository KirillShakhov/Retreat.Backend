package torrent

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
)

// TorrentManager управляет загрузкой и обработкой торрент-файлов
type TorrentManager struct {
	mu           sync.Mutex
	torrentInfo  map[string]*TorrentInfo
	filetypes    []string
	client       *torrent.Client
	downloadPath string
}

// TorrentInfo содержит информацию о загружаемом файле
type TorrentInfo struct {
	Id       string    `json:"id"`
	Name     string    `json:"name"`
	Progress int       `json:"progress"`
	Download bool      `json:"download"`
	Time     time.Time `json:"time"`
	File     *torrent.File
	Torrent  *torrent.Torrent
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
		torrentInfo:  make(map[string]*TorrentInfo),
		filetypes:    filetypes,
		client:       client,
		downloadPath: downloadPath,
	}
}

// AddTorrentFromFile добавляет торрент из файла
func (tm *TorrentManager) AddTorrentFromFile(torrentFile io.Reader, filename string) ([]string, error) {
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
	ids, err := tm.processTorrentFiles(t)
	if err != nil {
		t.Drop()
		return nil, err
	}

	return ids, nil
}

// processTorrentFiles обрабатывает файлы в добавленном торренте
func (tm *TorrentManager) processTorrentFiles(t *torrent.Torrent) ([]string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	ids := make([]string, 0)
	anyValid := false

	for _, f := range t.Files() {
		if !tm.isValidFile(f) {
			continue
		}

		id := tm.generateFileID(f)
		info, exists := tm.torrentInfo[id]

		if !exists {
			info = &TorrentInfo{
				Id:      id,
				Name:    f.DisplayPath(),
				Time:    time.Now(),
				File:    f,
				Torrent: t,
			}
			tm.torrentInfo[id] = info
		}

		ids = append(ids, info.Id)
		anyValid = true

		// Устанавливаем приоритеты для быстрого старта потоковой передачи
		tm.setStreamingPriorities(f)
	}

	if !anyValid {
		return nil, fmt.Errorf("no valid files in torrent %s", t.Name())
	}

	return ids, nil
}

// generateFileID генерирует уникальный ID для файла
func (tm *TorrentManager) generateFileID(f *torrent.File) string {
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
	tm.mu.Lock()
	defer tm.mu.Unlock()

	info, exists := tm.torrentInfo[id]
	if exists {
		// Обновляем прогресс
		info.Progress = tm.calculateProgress(info)
	}
	return info, exists
}

// GetTorrents возвращает список всех торрентов
func (tm *TorrentManager) GetTorrents() []*TorrentInfo {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	torrents := make([]*TorrentInfo, 0, len(tm.torrentInfo))
	for _, info := range tm.torrentInfo {
		info.Progress = tm.calculateProgress(info)
		torrents = append(torrents, info)
	}

	// Сортируем по времени добавления (новые сначала)
	sort.Slice(torrents, func(i, j int) bool {
		return torrents[i].Time.After(torrents[j].Time)
	})

	return torrents
}

// calculateProgress вычисляет прогресс загрузки файла
func (tm *TorrentManager) calculateProgress(info *TorrentInfo) int {
	if info.File == nil || info.File.Length() == 0 {
		return 0
	}
	return int(info.File.BytesCompleted() * 100 / info.File.Length())
}

func (tm *TorrentManager) RemoveTorrent(id string) (bool, string) {
	info, ok := tm.GetTorrent(id)
	if !ok {
		return true, "Torrent not found"
	}

	ih := info.File.Torrent().InfoHash().String()
	rel := info.File.Path()

	filePath := filepath.Join(tm.downloadPath, ih, rel)

	info.File.SetPriority(torrent.PiecePriorityNone)

	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.torrentInfo, id)

	err := os.Remove(filePath)
	if err != nil {
		return false, "Error removing file"
	}

	return true, "File removed"
}

// GetFilepath возвращает путь к загруженному файлу
func (tm *TorrentManager) GetFilepath(id string) (string, error) {
	info, exists := tm.GetTorrent(id)
	if !exists || info.File == nil {
		return "", fmt.Errorf("torrent not found: %s", id)
	}

	// В anacrolix/torrent файлы сохраняются в поддиректориях по infoHash
	torrentPath := filepath.Join(tm.downloadPath, info.Torrent.InfoHash().String())
	filename := filepath.Base(info.File.Path())

	return filepath.Join(torrentPath, filename), nil
}

// Close закрывает торрент-клиент
func (tm *TorrentManager) Close() {
	if tm.client != nil {
		tm.client.Close()
	}
}

func (tm *TorrentManager) AddMagnet(uri string) ([]string, error) {
	t, err := tm.client.AddMagnet(uri)
	if err != nil {
		return make([]string, 0), err
	}

	<-t.GotInfo()

	ids, err := tm.processTorrentFiles(t)
	if err != nil {
		t.Drop()
		return nil, err
	}

	if len(ids) <= 0 {
		t.Drop()
	}

	return ids, nil
}

func (tm *TorrentManager) getId(f *torrent.File) string {
	id := f.Torrent().InfoHash().String() + f.DisplayPath()
	return fmt.Sprintf("%x", md5.Sum([]byte(id)))
}

func convert(t *torrent.Torrent) *TorrentInfo {
	return &TorrentInfo{
		Id:      t.InfoHash().String(),
		Name:    t.Name(),
		Time:    time.Now(),
		File:    t.Files()[0],
		Torrent: t,
	}
}

func (tm *TorrentManager) AddTorrent(f *torrent.File) *TorrentInfo {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	id := tm.getId(f)

	info := TorrentInfo{
		Id:   id,
		Name: f.DisplayPath(),
		Time: time.Now(),
		File: f,
	}

	tm.torrentInfo[id] = &info
	return &info
}
