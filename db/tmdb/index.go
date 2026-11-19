package tmdb

import (
	"NUMParser/db/models"
	"NUMParser/db/storage"
	"NUMParser/db/utils"
	"log"
	"time"

	bolt "go.etcd.io/bbolt"
)

// NotFoundTTL: re-check torrents without TMDB match after 14 days
const NotFoundTTL = 14 * 24 * time.Hour

func GetIndex(hash string) int64 {
	if storage.DB == nil {
		return 0
	}
	var id int64
	storage.DB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("TMDB"))
		if bucket == nil {
			return nil
		}
		bucket = bucket.Bucket([]byte("Index"))
		if bucket == nil {
			return nil
		}
		buf := bucket.Get([]byte(hash))
		if len(buf) == 0 {
			return nil
		}
		id = utils.B2I(buf)
		return nil
	})
	return id
}

func SetIndex(t *models.TorrentDetails, e *models.Entity) {
	if storage.IsStopping() || storage.DB == nil || t == nil || e == nil {
		return
	}

	err := storage.DB.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("TMDB"))
		if err != nil {
			return err
		}
		if nf := bucket.Bucket([]byte("NotFound")); nf != nil {
			_ = nf.Delete([]byte(t.Hash))
		}
		bucket, err = bucket.CreateBucketIfNotExists([]byte("Index"))
		if err != nil {
			return err
		}
		return bucket.Put([]byte(t.Hash), utils.I2B(e.ID))
	})
	if err != nil {
		log.Println("Error write to db TMDB:", err)
	}
}

// IsNotFound returns true if hash was checked within NotFoundTTL and had no TMDB match
func IsNotFound(hash string) bool {
	if hash == "" || storage.DB == nil {
		return false
	}
	now := time.Now().Unix()
	var checkedAt int64
	storage.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("TMDB"))
		if b == nil {
			return nil
		}
		nf := b.Bucket([]byte("NotFound"))
		if nf == nil {
			return nil
		}
		buf := nf.Get([]byte(hash))
		if len(buf) > 0 {
			checkedAt = utils.B2I(buf)
		}
		return nil
	})

	if checkedAt != 0 {
		if now-checkedAt < int64(NotFoundTTL.Seconds()) {
			return true
		}
	}
	return false
}

// SetNotFound marks a torrent hash as checked with no TMDB match
func SetNotFound(hash string) {
	if storage.IsStopping() || storage.DB == nil || hash == "" {
		return
	}
	now := time.Now().Unix()

	_ = storage.DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("TMDB"))
		if err != nil {
			return err
		}
		nf, err := b.CreateBucketIfNotExists([]byte("NotFound"))
		if err != nil {
			return err
		}
		return nf.Put([]byte(hash), utils.I2B(now))
	})
}
