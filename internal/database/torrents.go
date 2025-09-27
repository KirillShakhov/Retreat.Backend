package database

import (
	"context"
	"errors"
	"retreat-backend/internal/torrent"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Torrent struct {
	ID          primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	Hash        string               `bson:"hash" json:"hash"`
	OwnerId     primitive.ObjectID   `bson:"owner_id" json:"owner_id"`
	TorrentFile string               `bson:"torrent_file" json:"torrent_file"`
	IsMagnet    bool                 `bson:"is_magnet" json:"is_magnet"`
	LastFileId  string               `bson:"last_file_id" json:"last_file_id"`
	CreatedAt   time.Time            `bson:"created_at" json:"created_at"`
	TorrentInfo *torrent.TorrentInfo `bson:"torrent_info" json:"torrent_info"`
}

type TorrentStore struct {
	mongodb *MongoDB
}

func NewTorrentStore(mongodb *MongoDB) *TorrentStore {
	return &TorrentStore{
		mongodb: mongodb,
	}
}

func (ts *TorrentStore) CreateTorrent(ownerId primitive.ObjectID, torrentInfo *torrent.TorrentInfo, torrentFile string, isMagnet bool) error {

	collection := ts.mongodb.GetCollection("torrents")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var existingTorrent Torrent
	err := collection.FindOne(ctx, bson.M{"ownerId": ownerId, "hash": torrentInfo.Id}).Decode(&existingTorrent)
	if err == nil {
		return errors.New("torrent already exists")
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}

	torrent := Torrent{
		OwnerId:     ownerId,
		Hash:        torrentInfo.Id,
		TorrentFile: torrentFile,
		IsMagnet:    isMagnet,
		CreatedAt:   time.Now(),
		TorrentInfo: torrentInfo,
	}

	_, err = collection.InsertOne(ctx, torrent)
	return err
}

func (ts *TorrentStore) GetTorrents(ownerId primitive.ObjectID) ([]*Torrent, error) {
	collection := ts.mongodb.GetCollection("torrents")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := collection.Find(ctx, bson.M{"owner_id": ownerId})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var torrents []*Torrent
	// cursor.All() загружает все результаты в память сразу.
	// Это может быть проблемой при большом количестве данных.
	if err = cursor.All(ctx, &torrents); err != nil {
		return nil, err
	}

	return torrents, nil
}
