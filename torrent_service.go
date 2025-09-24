package main

import (
	"crypto/md5"
	"fmt"
	"path"
	"slices"
	"sort"
	"time"

	"github.com/anacrolix/torrent"
)

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
