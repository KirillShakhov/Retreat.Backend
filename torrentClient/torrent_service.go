package torrentClient

import (
	"crypto/md5"
	"fmt"
	"path"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
)

type TorrentInfo struct {
	Id       string    `json:"id"`
	Name     string    `json:"name"`
	Progress int       `json:"progress"`
	Download bool      `json:"download"`
	Time     time.Time `json:"time"`
	File     *torrent.File
}

type TorrentClient struct {
	mu          sync.Mutex
	torrentInfo map[string]*TorrentInfo
	Filetypes   []string `json:"filetypes"`
}

func (tc *TorrentClient) GetId(f *torrent.File) string {
	id := f.Torrent().InfoHash().String() + f.DisplayPath()
	return fmt.Sprintf("%x", md5.Sum([]byte(id)))
}

func (tc *TorrentClient) AddTorrent(f *torrent.File) *TorrentInfo {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	id := tc.GetId(f)

	info := TorrentInfo{
		Id:   id,
		Name: f.DisplayPath(),
		Time: time.Now(),
		File: f,
	}

	tc.torrentInfo[id] = &info
	return &info
}

func (tc *TorrentClient) GetTorrent(id string) (*TorrentInfo, bool) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	t, ok := tc.torrentInfo[id]
	return t, ok
}

func (tc *TorrentClient) GetTorrents() []*TorrentInfo {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	torrents := make([]*TorrentInfo, 0, len(tc.torrentInfo))
	for _, info := range tc.torrentInfo {
		info.Progress = int(info.File.BytesCompleted() * 100 / info.File.Length())
		torrents = append(torrents, info)
	}

	sort.Slice(torrents, func(i, j int) bool {
		return torrents[i].Time.After(torrents[j].Time)
	})

	return torrents
}

func (tc *TorrentClient) RemoveTorrent(id string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	delete(tc.torrentInfo, id)
}

func (tc *TorrentClient) IsValidFile(f *torrent.File) bool {
	ext := path.Ext(f.Path())
	return slices.Contains(tc.Filetypes, ext)
}
